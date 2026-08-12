package main

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// GameState is which phase of the game loop Game.Update/Draw is
// currently driving. Exactly one is active at a time; every
// transition between them is made explicit in the updateX methods
// below rather than inferred from ad hoc flags scattered across
// Game's fields.
type GameState int

const (
	// StateAttract shows the title screen over a decorative,
	// non-interactive asteroid field and waits for Enter to start a
	// game (see updateAttract/startGame).
	StateAttract GameState = iota

	// StatePlaying is normal gameplay: ship, rocks, shots, and
	// collisions are all live (see updatePlaying).
	StatePlaying

	// StateWaveClear is the brief pause after every rock in a wave
	// is destroyed, before the next wave spawns (see
	// updateWaveClear).
	StateWaveClear

	// StatePlayerDying is the pause after the ship is destroyed --
	// its death burst plays out, rocks keep drifting, and input is
	// frozen -- before it either respawns or the game ends (see
	// updatePlayerDying).
	StatePlayerDying

	// StateGameOver holds on a game-over screen until Enter returns
	// to StateAttract (see updateGameOver).
	StateGameOver
)

// startingLives is how many lives a fresh game begins with.
const startingLives = 3

// maxWaveAsteroids caps how many Large rocks a wave can start with,
// so the field doesn't grow forever as waves climb -- mirrors the
// arcade original's plateau (see asteroidsForWave).
const maxWaveAsteroids = 22

// waveClearPause is how long the field sits empty -- ship still
// flying, nothing left to shoot -- before the next wave spawns.
const waveClearPause = 2.0 // seconds

// playerDeathPause is how long the ship stays destroyed -- its death
// burst playing out -- before it respawns (or the game ends, if that
// was the last life).
const playerDeathPause = 1.5 // seconds

// beatIntervalMax/Min bound the heartbeat tempo (see
// currentBeatInterval): it starts slow when a wave is full of rocks
// and speeds up toward Min as the wave clears out, mirroring the
// arcade original's escalating tension.
const beatIntervalMax = 1.0  // seconds, at wave start
const beatIntervalMin = 0.25 // seconds, with the wave almost cleared

// extraLifeScore is how many points earn the player a bonus life --
// every multiple of this threshold the score crosses grants one (see
// awardExtraLives).
const extraLifeScore = 10000

// --- Saucer spawn/aim policy ---
//
// Saucer itself (saucer.go) only knows how to move and how to fire in
// a direction it's handed -- everything about *when* one appears,
// *which kind*, and *how well it aims* is policy that depends on the
// player's score, so it lives here alongside the rest of Game's
// decision-making rather than in the entity. Consulted from the
// original arcade's disassembly/RAM map (see saucer.go's doc comment
// for links) rather than guessed at.

// saucerSpawnIntervalMin/Max bound how long the game waits, once no
// saucer is present, before the next one appears.
const saucerSpawnIntervalMin = 12.0
const saucerSpawnIntervalMax = 22.0

// smallSaucerScore is the score after which small saucers start
// appearing at all -- 10,000 in the original arcade. Below it, every
// saucer spawned is Large.
const smallSaucerScore = 10000

// smallSaucerBaseChance/MaxChance/ChanceRamp shape how likely a
// newly-spawned saucer is Small once smallSaucerScore is reached: it
// starts at smallSaucerBaseChance and climbs toward smallSaucerMaxChance
// as score increases, gaining 100% chance per smallSaucerChanceRamp
// points -- the original's small saucers "appear more frequently" as
// the game progresses, without documenting an exact formula for it,
// so this is a reasonable ramp rather than a ROM-accurate one.
const smallSaucerBaseChance = 0.3
const smallSaucerMaxChance = 0.8
const smallSaucerChanceRamp = 20000

// smallSaucerAccurateScore is the score after which the small
// saucer's aim sharpens up -- 35,000 in the original arcade.
const smallSaucerAccurateScore = 35000

// smallSaucerAimJitterWide/Tight bound how far off-target (in
// radians) the small saucer's shot can land before/after
// smallSaucerAccurateScore. The original never lets even an
// "accurate" small saucer aim perfectly, and doesn't lead the
// player's movement at all -- so this jitters the direction straight
// at the player's current position rather than converging to a
// zero-error aimbot.
const smallSaucerAimJitterWide = 0.35  // ~20 degrees
const smallSaucerAimJitterTight = 0.08 // ~4.5 degrees

// randomSaucerSpawnDelay picks how long to wait before the next
// saucer, once none is present (see Game.despawnSaucer).
func randomSaucerSpawnDelay() float64 {
	return saucerSpawnIntervalMin + rand.Float64()*(saucerSpawnIntervalMax-saucerSpawnIntervalMin)
}

// smallSaucerChance returns the odds (0..1) that the next saucer
// spawned should be Small rather than Large, given the player's
// current score (see smallSaucerBaseChance/MaxChance/ChanceRamp).
func smallSaucerChance(score int) float64 {
	if score < smallSaucerScore {
		return 0
	}
	chance := smallSaucerBaseChance + float64(score-smallSaucerScore)/smallSaucerChanceRamp
	if chance > smallSaucerMaxChance {
		chance = smallSaucerMaxChance
	}
	return chance
}

// saucerAimDirection returns the direction a saucer at pos should
// fire in to hit target (the player's ship). Large saucers ignore the
// player entirely and fire in a random direction; Small saucers aim
// at the player's current position, jittered by an angle that
// narrows once score passes smallSaucerAccurateScore (see
// smallSaucerAimJitterWide/Tight) -- it aims where the player is
// *right now*, same as the original, not where they're headed.
func saucerAimDirection(kind SaucerKind, pos, target Point, score int) Vector {
	if kind == SaucerLarge {
		return FromAngle(rand.Float64() * 2 * math.Pi)
	}

	base := math.Atan2(target.Y-pos.Y, target.X-pos.X)
	jitter := smallSaucerAimJitterWide
	if score >= smallSaucerAccurateScore {
		jitter = smallSaucerAimJitterTight
	}
	base += (rand.Float64()*2 - 1) * jitter
	return FromAngle(base)
}

// asteroidsForWave returns how many Large rocks wave should start
// with: startingAsteroids on wave 1, ramping up by two per wave until
// maxWaveAsteroids.
func asteroidsForWave(wave int) int {
	n := startingAsteroids + ((wave - 1) * 2)
	if n > maxWaveAsteroids {
		n = maxWaveAsteroids
	}
	return n
}

// startGame resets lives/wave state, drops the ship back at screen
// center with a fresh invulnerability window, and starts wave 1 --
// called when Enter is pressed from the attract screen.
func (g *Game) startGame() {
	g.lives = startingLives
	g.wave = 1
	g.score = 0
	g.playerShip.respawn()
	g.playerShip.Alive = true
	g.playerShip.grantInvulnerability()
	g.explosions = nil
	g.beatToggle = false
	g.beatTimer = 0
	g.saucer = nil
	g.saucerTimer = randomSaucerSpawnDelay()
	g.startWave()
	g.state = StatePlaying
}

// startWave spawns the next wave's rock field (kept clear of the
// ship's current position, same guarantee the very first wave gets)
// and records how many rocks it started with, which drives the
// heartbeat's tempo ramp (see currentBeatInterval).
func (g *Game) startWave() {
	g.waveRockCount = asteroidsForWave(g.wave)
	g.asteroids = spawnAsteroidField(g.waveRockCount, g.playerShip.Pos)
}

// despawnSaucer removes the current saucer -- however it went
// (destroyed, escaped off-screen, or the round ending under it) --
// and restarts the countdown to the next one.
func (g *Game) despawnSaucer() {
	g.saucer = nil
	g.saucerTimer = randomSaucerSpawnDelay()
}

// updateSaucerSpawn counts down to the next saucer while none is
// present, and spawns one (see smallSaucerChance for the Large/Small
// pick) once the timer runs out. No-ops while a saucer is already on
// screen -- only one is ever live at a time, same as the original.
func (g *Game) updateSaucerSpawn(dt float64) {
	if g.saucer != nil {
		return
	}
	g.saucerTimer -= dt
	if g.saucerTimer > 0 {
		return
	}
	kind := SaucerLarge
	if rand.Float64() < smallSaucerChance(g.score) {
		kind = SaucerSmall
	}
	g.saucer = NewSaucer(kind, g.playerShip.Pos)
}

// updateSaucer drives the current saucer, if any: movement, firing
// (Game picks the aim direction -- see saucerAimDirection -- since
// that needs the player's position and score, which Saucer itself
// doesn't know), and every kind of collision it's party to except
// the player's own shots hitting it (that's handled in updatePlaying,
// alongside the player's shots-vs-rocks collision it's a sibling of).
// Called from both updatePlaying and updateWaveClear -- a saucer that
// showed up right as the last rock died shouldn't just hang there
// frozen (still humming) through the wave-clear pause, same as the
// ship and its shots don't freeze either. It only ever fully stops
// during StatePlayerDying/StateGameOver/StateAttract, none of which
// call this.
func (g *Game) updateSaucer(dt float64) {
	if g.saucer == nil {
		return
	}

	g.saucer.Update(dt)

	if g.saucer.Escaped() {
		g.despawnSaucer()
		return
	}

	if g.saucer.ReadyToFire {
		dir := saucerAimDirection(g.saucer.Kind, g.saucer.Pos, g.playerShip.Pos, g.score)
		g.saucer.fire(dir)
		// No dedicated saucer-fire sound yet -- reuse the player's
		// own fire cue, same "no dedicated sound, reuse the closest
		// existing one" call killPlayer already makes for a
		// ship-death bang.
		g.soundManager.Play(SFXFire)
	}

	// The saucer's own shots can pop rocks too, same rules as the
	// player's shots -- just with no score attached (only the player
	// earns points).
	var saucerShotExplosions []*Explosion
	g.saucer.Shots, g.asteroids, saucerShotExplosions, _ = CheckShotRockCollisions(g.saucer.Shots, g.asteroids, g.soundManager)
	g.explosions = append(g.explosions, saucerShotExplosions...)

	// A rock colliding with the saucer destroys it -- no score, since
	// the player didn't do it -- same as the original arcade.
	if CheckRockSaucerCollision(g.asteroids, g.saucer) {
		g.soundManager.Play(bangSFXForSaucer(g.saucer.Kind))
		g.explosions = append(g.explosions, NewExplosion(g.saucer.Pos))
		g.despawnSaucer()
		return
	}

	// Either the saucer's shots or its body killing the player is
	// handled identically to a rock doing it -- the saucer itself
	// isn't touched by a body collision, mirroring how
	// CheckShipRockCollision leaves the rock alone too.
	if CheckShotShipCollision(g.saucer.Shots, g.playerShip) || CheckShipSaucerCollision(g.playerShip, g.saucer) {
		g.killPlayer()
	}
}

// killPlayer marks the ship destroyed, spawns its death burst, and
// hands off to StatePlayerDying -- input freezes and the ship goes
// invisible until the death pause elapses (see updatePlayerDying).
// Shared by a rock collision and a fatal hyperspace jump (see
// checkHyperspaceDeath) so both kinds of death are handled
// identically from here on.
func (g *Game) killPlayer() {
	g.soundManager.Play(SFXBangLarge)
	g.soundManager.StopLoop(SFXThrustLoop)
	g.explosions = append(g.explosions, NewShipExplosion(g.playerShip.Pos))
	g.playerShip.Alive = false
	g.state = StatePlayerDying
	g.stateTimer = playerDeathPause
}

// awardExtraLives grants one life for every multiple of
// extraLifeScore the player's score just crossed -- prevScore and
// g.score (the score before and after this tick's gain) bracket the
// jump, so a single big gain (e.g. two rocks popped in one tick)
// can't skip past a threshold without awarding it.
func (g *Game) awardExtraLives(prevScore int) {
	before := prevScore / extraLifeScore
	after := g.score / extraLifeScore
	if after <= before {
		return
	}
	g.lives += after - before
	g.soundManager.Play(SFXExtraShip)
}

// checkHyperspaceDeath routes a fatal hyperspace return through
// killPlayer. Called right after playerShip.Update so a jump that
// resolves fatally this tick hands off to StatePlayerDying
// immediately -- whether or not any rocks are currently in play (see
// updateWaveClear, which also calls this).
func (g *Game) checkHyperspaceDeath() {
	if g.playerShip.HyperspaceDestroyed {
		g.killPlayer()
	}
}

// playFireSFXIfFired plays the fire sound on any tick the ship just
// spawned a new shot. Shared by updatePlaying and updateWaveClear,
// since the ship can still fire (at nothing) during the wave-clear
// pause.
func (g *Game) playFireSFXIfFired() {
	if g.playerShip.Fired {
		g.soundManager.Play(SFXFire)
	}
}

// updateExplosions ages every in-progress spark burst and drops any
// that have finished, compacting the slice in place -- same
// compact-in-place pattern as PlayerShip.updateBullets.
func (g *Game) updateExplosions(dt float64) {
	live := g.explosions[:0]
	for _, e := range g.explosions {
		e.Update(dt)
		if !e.Expired() {
			live = append(live, e)
		}
	}
	g.explosions = live
}

// updateLoops starts/stops the thrust hum to match the ship's own
// Thrusting state, and the size-matched saucer hum to match whether
// (and which kind of) saucer is currently present -- both driven off
// existing state rather than polling anything again here, so sound
// and what's on screen can never disagree.
func (g *Game) updateLoops() {
	if g.playerShip.Thrusting {
		g.soundManager.PlayLoop(SFXThrustLoop)
	} else {
		g.soundManager.StopLoop(SFXThrustLoop)
	}

	switch {
	case g.saucer == nil:
		g.soundManager.StopLoop(SFXSaucerBigLoop)
		g.soundManager.StopLoop(SFXSaucerSmallLoop)
	case g.saucer.Kind == SaucerLarge:
		g.soundManager.PlayLoop(SFXSaucerBigLoop)
		g.soundManager.StopLoop(SFXSaucerSmallLoop)
	default:
		g.soundManager.PlayLoop(SFXSaucerSmallLoop)
		g.soundManager.StopLoop(SFXSaucerBigLoop)
	}
}

// updateHeartbeat ticks the alternating beat1/beat2 heartbeat at
// currentBeatInterval's tempo. Only called while StatePlaying, so the
// heartbeat goes quiet whenever there's no active wave threatening
// the player.
func (g *Game) updateHeartbeat(dt float64) {
	g.beatTimer -= dt
	if g.beatTimer > 0 {
		return
	}
	if g.beatToggle {
		g.soundManager.Play(SFXBeat2)
	} else {
		g.soundManager.Play(SFXBeat1)
	}
	g.beatToggle = !g.beatToggle
	g.beatTimer += g.currentBeatInterval()
}

// currentBeatInterval interpolates between beatIntervalMax and
// beatIntervalMin based on how much of the wave's starting rock count
// is still on screen -- a full field ticks slow, a wave down to its
// last rock or two ticks fast. A split can briefly push the live
// count above the wave's starting count (a Large rock becomes two
// Medium ones); frac is clamped to 1 so that doesn't overshoot
// beatIntervalMax.
func (g *Game) currentBeatInterval() float64 {
	if g.waveRockCount <= 0 {
		return beatIntervalMax
	}
	frac := float64(len(g.asteroids)) / float64(g.waveRockCount)
	if frac > 1 {
		frac = 1
	}
	return beatIntervalMin + frac*(beatIntervalMax-beatIntervalMin)
}

// updateDebugKeys drives the manual sound-test keys. These aren't
// gameplay, so they stay live in every state rather than being gated
// by it, unlike the keys in Settings.Keys. Parked on F1-F5 rather
// than the number row since 0-9 is now the player-facing volume
// control (see updateVolumeKeys).
func (g *Game) updateDebugKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.soundManager.Play(SFXBangSmall)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		g.soundManager.Play(SFXBangMedium)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.soundManager.Play(SFXBangLarge)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
		g.soundManager.Play(SFXExtraShip)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		if g.soundManager.IsLoopPlaying(SFXSaucerBigLoop) {
			g.soundManager.StopLoop(SFXSaucerBigLoop)
		} else {
			g.soundManager.PlayLoop(SFXSaucerBigLoop)
		}
	}
}

// volumeKeys maps each Key0..Key9 to the 0-9 level it sets (see
// updateVolumeKeys) -- spelled out explicitly rather than derived by
// arithmetic on ebiten.Key0, since ebiten doesn't guarantee its Key
// constants are numbered in a way that's safe to offset.
var volumeKeys = [10]ebiten.Key{
	ebiten.Key0, ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4,
	ebiten.Key5, ebiten.Key6, ebiten.Key7, ebiten.Key8, ebiten.Key9,
}

// updateVolumeKeys handles 0-9, the player-facing volume control (see
// SoundManager.SetVolumeLevel) -- 0 is silent, 9 is full volume.
// Live in every state, not just StateAttract where the prompt is
// drawn, so volume can be adjusted mid-game too.
func (g *Game) updateVolumeKeys() {
	for level, key := range volumeKeys {
		if inpututil.IsKeyJustPressed(key) {
			g.soundManager.SetVolumeLevel(level)
		}
	}
}

// updateAttract drifts the decorative asteroid field shown behind the
// title screen and waits for Enter (or a tap, on a touchscreen) to
// start a fresh game. A tap here is also the one-time signal that
// switches the on-screen controls on for the rest of the session (see
// TouchInput) -- once g.touchControls is set it stays set, so a
// hybrid device that later plugs in a keyboard doesn't lose the
// overlay.
func (g *Game) updateAttract(dt float64) {
	for _, a := range g.asteroids {
		a.Update(dt)
	}
	tapped := touchJustPressed()
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || tapped {
		if tapped {
			g.enableTouchControls()
		}
		g.startGame()
	}
}

// enableTouchControls turns the on-screen controls on (see the
// touchControls field doc) and, the first time it's called, switches
// the canvas from its default landscape aspect to the narrower
// touchScreenWidth/Height portrait aspect and relays out the touch
// buttons to match (see layoutTouchButtons). Safe to call more than
// once -- only the first call changes anything, matching
// touchControls' own "set once, stays set" rule.
func (g *Game) enableTouchControls() {
	if g.touchControls {
		return
	}
	g.touchControls = true
	screenWidth = touchScreenWidth
	screenHeight = touchScreenHeight
	ebiten.SetWindowSize(int(screenWidth), int(screenHeight))
	layoutTouchButtons()
}

// updatePlaying runs normal gameplay: ship/rock/shot simulation,
// collision handling, the heartbeat, and the transitions out of
// Playing into StateWaveClear or StatePlayerDying.
func (g *Game) updatePlaying(dt float64) {
	g.playerShip.Update(dt, currentTouchInput())
	g.checkHyperspaceDeath()
	if g.state != StatePlaying {
		// A fatal hyperspace jump just handed off to
		// StatePlayerDying -- nothing else this tick belongs to a
		// Playing update.
		return
	}

	for _, a := range g.asteroids {
		a.Update(dt)
	}
	g.updateSaucerSpawn(dt)
	g.updateSaucer(dt)
	if g.state != StatePlaying {
		// The saucer's shots or its body just killed the player --
		// same reasoning as the checkHyperspaceDeath guard above:
		// nothing else this tick belongs to a Playing update.
		return
	}

	var newExplosions []*Explosion
	var scoreGained int
	g.playerShip.Shots, g.asteroids, newExplosions, scoreGained = CheckShotRockCollisions(g.playerShip.Shots, g.asteroids, g.soundManager)
	g.explosions = append(g.explosions, newExplosions...)

	// Player shots vs the saucer, if one's currently around --
	// sibling of the shots-vs-rocks check just above, kept separate
	// since a saucer isn't a slice of things to range over.
	if g.saucer != nil {
		var hit bool
		var hitPos Point
		g.playerShip.Shots, hit, hitPos = CheckShotSaucerCollision(g.playerShip.Shots, g.saucer)
		if hit {
			g.soundManager.Play(bangSFXForSaucer(g.saucer.Kind))
			g.explosions = append(g.explosions, NewExplosion(hitPos))
			scoreGained += saucerScore[g.saucer.Kind]
			g.despawnSaucer()
		}
	}

	if scoreGained > 0 {
		prevScore := g.score
		g.score += scoreGained
		g.awardExtraLives(prevScore)
	}

	if CheckShipRockCollision(g.playerShip, g.asteroids) {
		g.killPlayer()
		return
	}

	g.updateExplosions(dt)
	g.updateHeartbeat(dt)
	g.playFireSFXIfFired()
	g.updateLoops()

	// Wave cleared: every rock destroyed hands off to a brief pause
	// before the next wave spawns (see updateWaveClear).
	if len(g.asteroids) == 0 {
		g.state = StateWaveClear
		g.stateTimer = waveClearPause
	}
}

// updateWaveClear keeps the ship (and any shots/explosions) animating
// during the pause between waves -- only the rocks are gone -- then
// spawns the next wave once stateTimer runs out.
func (g *Game) updateWaveClear(dt float64) {
	g.playerShip.Update(dt, currentTouchInput())
	g.checkHyperspaceDeath()
	if g.state != StateWaveClear {
		return
	}

	// A saucer that was already on screen when the wave cleared keeps
	// flying/firing through the pause -- see updateSaucer's doc
	// comment. No updateSaucerSpawn here, though: a new one shouldn't
	// pop in during this brief gap between waves.
	g.updateSaucer(dt)
	if g.state != StateWaveClear {
		// The saucer's shots or its body just killed the player.
		return
	}

	g.updateExplosions(dt)
	g.playFireSFXIfFired()
	g.updateLoops()

	g.stateTimer -= dt
	if g.stateTimer <= 0 {
		g.wave++
		g.startWave()
		g.state = StatePlaying
	}
}

// updatePlayerDying freezes ship control while its death burst plays
// out -- rocks keep drifting and any shots already in flight keep
// going, so the field doesn't look paused -- then either respawns the
// ship (with a fresh invulnerability window) or, if that was the last
// life, hands off to StateGameOver.
func (g *Game) updatePlayerDying(dt float64) {
	for _, a := range g.asteroids {
		a.Update(dt)
	}
	g.playerShip.updateBullets(dt)
	g.updateExplosions(dt)

	g.stateTimer -= dt
	if g.stateTimer > 0 {
		return
	}

	g.lives--
	if g.lives <= 0 {
		g.state = StateGameOver
		return
	}

	g.playerShip.respawn()
	g.playerShip.Alive = true
	g.playerShip.grantInvulnerability()
	g.state = StatePlaying
}

// updateGameOver holds on the game-over screen, still drifting
// whatever rocks are left for visual continuity, until Enter returns
// to the attract screen with a fresh decorative field.
func (g *Game) updateGameOver(dt float64) {
	for _, a := range g.asteroids {
		a.Update(dt)
	}
	g.updateExplosions(dt)
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || touchJustPressed() {
		g.asteroids = spawnAsteroidField(startingAsteroids, g.playerShip.Pos)
		g.saucer = nil
		g.soundManager.StopLoop(SFXSaucerBigLoop)
		g.soundManager.StopLoop(SFXSaucerSmallLoop)
		g.state = StateAttract
	}
}
