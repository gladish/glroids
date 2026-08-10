package main

import (
	"image/color"
	"log"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// portrait 4x3
const screenWidth = 1000
const screenHeight = 750

// startingAsteroids is how many Large rocks spawn at the start of a
// wave.
const startingAsteroids = 5

// asteroidSpawnClearance is how far from the ship's starting position
// a freshly spawned asteroid must be, so a wave never starts with a
// rock landing on top of the player.
const asteroidSpawnClearance = 200.0

// beatInterval is the fixed gap between heartbeat clicks. Real
// Asteroids speeds this up as a wave clears; until there's a state
// engine to drive that, it just ticks at a constant tempo.
const beatInterval = 0.6 // seconds

type Game struct {
	settings     Settings
	soundManager *SoundManager
	playerShip   *PlayerShip
	asteroids    []*Rock
	explosions   []*Explosion // short-lived spark bursts, one per rock hit
	beatToggle   bool         // alternates SFXBeat1/SFXBeat2 each tick
	beatTimer    float64      // seconds until the next heartbeat tick
}

func NewGame() *Game {
	audioContext := audio.NewContext(sampleRate)
	settings := DefaultSettings()
	shipStart := Point{X: screenWidth / 2, Y: screenHeight / 2}
	return &Game{
		settings:     settings,
		soundManager: NewSoundManager(audioContext),
		playerShip:   NewPlayerShip(shipStart, settings),
		asteroids:    spawnAsteroidField(startingAsteroids, shipStart),
	}
}

// spawnAsteroidField creates n Large asteroids of random style at
// random positions, each kept at least asteroidSpawnClearance away
// from avoid (the ship's start point).
func spawnAsteroidField(n int, avoid Point) []*Rock {
	field := make([]*Rock, 0, n)
	for len(field) < n {
		pos := Point{X: rand.Float64() * screenWidth, Y: rand.Float64() * screenHeight}
		if pos.Sub(avoid).Length() < asteroidSpawnClearance {
			continue
		}
		field = append(field, NewRandomAsteroid(pos))
	}
	return field
}

func (g *Game) Update() error {
	// Advances any in-progress loop fade-outs (see StopLoop).
	g.soundManager.Update()

	dt := 1.0 / float64(ebiten.TPS())
	g.playerShip.Update(dt)
	for _, a := range g.asteroids {
		a.Update(dt)
	}

	// A rock hit this tick is replaced by its split children (or
	// removed outright, if it was Small) before anything else reads
	// g.asteroids, so Draw never sees a rock that was already
	// destroyed this frame. Any spark bursts spawned by a hit start
	// this same tick at age 0 -- Draw is fine seeing them mid-burst
	// right away, unlike a fresh Rock they don't need any state
	// precomputed before their first Update.
	var newExplosions []*Explosion
	g.playerShip.Shots, g.asteroids, newExplosions = CheckShotRockCollisions(g.playerShip.Shots, g.asteroids, g.soundManager)
	g.explosions = append(g.explosions, newExplosions...)

	// Age out finished bursts each tick, same compact-in-place pattern
	// as PlayerShip.updateBullets.
	liveExplosions := g.explosions[:0]
	for _, e := range g.explosions {
		e.Update(dt)
		if !e.Expired() {
			liveExplosions = append(liveExplosions, e)
		}
	}
	g.explosions = liveExplosions

	// One-shots: each press pulls the next idle voice from the pool,
	// so mashing the key overlaps cleanly instead of cutting the
	// previous sound off. Fire is driven off the ship's own Fired
	// flag rather than the raw key, so mashing Space past the 3-shot
	// cap doesn't play a sound for a shot that never spawned.
	if g.playerShip.Fired {
		g.soundManager.Play(SFXFire)
	}
	if g.playerShip.HyperspaceDestroyed {
		g.soundManager.Play(SFXBangLarge)
	}
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

	// Heartbeat: always ticking, alternating beat1/beat2, at a fixed
	// tempo. Not gated on any game state yet -- once a wave/state
	// engine exists, it should own beatInterval (and start/stop) so
	// the tempo can ramp as a wave clears.
	g.beatTimer -= dt
	if g.beatTimer <= 0 {
		if g.beatToggle {
			g.soundManager.Play(SFXBeat2)
		} else {
			g.soundManager.Play(SFXBeat1)
		}
		g.beatToggle = !g.beatToggle
		g.beatTimer += beatInterval
	}

	// Loops: start while the ship is thrusting, stop the instant it
	// isn't. Driven off the ship's own state rather than polling the
	// key again here, so sound and motion can never disagree.
	if g.playerShip.Thrusting {
		g.soundManager.PlayLoop(SFXThrustLoop)
	} else {
		g.soundManager.StopLoop(SFXThrustLoop)
	}

	// Toggle-style loop: press once to start the saucer hum, press
	// again to stop it.
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if g.soundManager.IsLoopPlaying(SFXSaucerBigLoop) {
			g.soundManager.StopLoop(SFXSaucerBigLoop)
		} else {
			g.soundManager.PlayLoop(SFXSaucerBigLoop)
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	g.playerShip.Draw(screen)
	for _, a := range g.asteroids {
		a.Draw(screen)
	}
	for _, e := range g.explosions {
		e.Draw(screen)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// World space is a fixed screenWidth x screenHeight canvas, matching
	// SetWindowSize 1:1 -- one path/position unit is one screen pixel,
	// no implicit stretch factor to account for.
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("glroids")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
