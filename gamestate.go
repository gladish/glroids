package main

import (
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
const maxWaveAsteroids = 11

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

// asteroidsForWave returns how many Large rocks wave should start
// with: startingAsteroids on wave 1, ramping up by one per wave until
// maxWaveAsteroids.
func asteroidsForWave(wave int) int {
	n := startingAsteroids + (wave - 1)
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
	g.playerShip.respawn()
	g.playerShip.Alive = true
	g.playerShip.grantInvulnerability()
	g.explosions = nil
	g.beatToggle = false
	g.beatTimer = 0
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
// Thrusting state -- driven off the ship rather than polling the key
// again here, so sound and motion can never disagree.
func (g *Game) updateLoops() {
	if g.playerShip.Thrusting {
		g.soundManager.PlayLoop(SFXThrustLoop)
	} else {
		g.soundManager.StopLoop(SFXThrustLoop)
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
// by it, unlike the keys in Settings.Keys.
func (g *Game) updateDebugKeys() {
	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		g.soundManager.Play(SFXBangSmall)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		g.soundManager.Play(SFXBangMedium)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key3) {
		g.soundManager.Play(SFXBangLarge)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key4) {
		g.soundManager.Play(SFXExtraShip)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if g.soundManager.IsLoopPlaying(SFXSaucerBigLoop) {
			g.soundManager.StopLoop(SFXSaucerBigLoop)
		} else {
			g.soundManager.PlayLoop(SFXSaucerBigLoop)
		}
	}
}

// updateAttract drifts the decorative asteroid field shown behind the
// title screen and waits for Enter to start a fresh game.
func (g *Game) updateAttract(dt float64) {
	for _, a := range g.asteroids {
		a.Update(dt)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.startGame()
	}
}

// updatePlaying runs normal gameplay: ship/rock/shot simulation,
// collision handling, the heartbeat, and the transitions out of
// Playing into StateWaveClear or StatePlayerDying.
func (g *Game) updatePlaying(dt float64) {
	g.playerShip.Update(dt)
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

	var newExplosions []*Explosion
	g.playerShip.Shots, g.asteroids, newExplosions = CheckShotRockCollisions(g.playerShip.Shots, g.asteroids, g.soundManager)
	g.explosions = append(g.explosions, newExplosions...)

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
	g.playerShip.Update(dt)
	g.checkHyperspaceDeath()
	if g.state != StateWaveClear {
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
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.asteroids = spawnAsteroidField(startingAsteroids, g.playerShip.Pos)
		g.state = StateAttract
	}
}
