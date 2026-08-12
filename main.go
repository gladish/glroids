package main

import (
	"fmt"
	"image/color"
	"log"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
)

// landscapeScreenWidth/Height is the default canvas, a 4:3 landscape
// aspect. touchScreenWidth/Height is the narrower 3:4 portrait aspect
// the canvas switches to the first time a touch is seen (see
// Game.enableTouchControls) -- friendlier for a phone held upright
// with the on-screen controls anchored to the bottom.
const (
	landscapeScreenWidth  = 1000.0
	landscapeScreenHeight = 750.0
	touchScreenWidth      = 750.0
	touchScreenHeight     = 1000.0
)

// screenWidth/screenHeight are the canvas's current logical size --
// every screen-relative calculation in the game (spawn positions,
// wrapping, HUD placement, Layout) reads these rather than a fixed
// constant, so the canvas can change aspect ratio at runtime. They
// start at the landscape default and only ever change once, the
// first time a touch is seen (see Game.enableTouchControls).
var (
	screenWidth  = landscapeScreenWidth
	screenHeight = landscapeScreenHeight
)

// startingAsteroids is how many Large rocks spawn at the start of
// wave 1 (see asteroidsForWave for how later waves scale up from
// this).
const startingAsteroids = 5

// asteroidSpawnClearance is how far from the ship's current position
// a freshly spawned asteroid must be, so a wave never starts with a
// rock landing on top of the player.
const asteroidSpawnClearance = 200.0

// Game holds everything driving one running instance of glroids: the
// live entities (ship/rocks/shots/explosions), audio, settings, and
// the state machine (see gamestate.go) that decides which of those
// entities are even live right now.
type Game struct {
	settings     Settings
	soundManager *SoundManager
	playerShip   *PlayerShip
	asteroids    []*Rock
	explosions   []*Explosion // short-lived spark bursts, one per rock hit
	beatToggle   bool         // alternates SFXBeat1/SFXBeat2 each tick
	beatTimer    float64      // seconds until the next heartbeat tick

	// state is which phase of the game loop is currently driving
	// Update/Draw -- see GameState and each updateX method in
	// gamestate.go for what runs in each one and how they transition.
	state GameState

	// stateTimer counts down a state's pause, for the states that
	// have one (StateWaveClear, StatePlayerDying) -- unused
	// otherwise.
	stateTimer float64

	lives int // remaining lives, including the ship currently in play
	wave  int // current wave number, starting at 1
	score int // current score -- see rockScore/extraLifeScore

	// waveRockCount is how many rocks the current wave started with
	// -- drives the heartbeat's tempo ramp (see currentBeatInterval).
	waveRockCount int

	// saucer is the current enemy saucer, or nil when none is on
	// screen -- only one is ever live at a time (see
	// updateSaucerSpawn). saucerTimer counts down to the next one
	// while saucer is nil; it's meaningless while one is already
	// present.
	saucer      *Saucer
	saucerTimer float64

	// touchControls is true once a tap has been seen on the title
	// screen (see updateAttract) -- it means this session is on a
	// touchscreen, so Draw keeps the on-screen buttons (see
	// drawTouchControls) showing for the rest of the run. Never reset
	// back to false once set.
	touchControls bool
}

func NewGame() *Game {
	audioContext := audio.NewContext(sampleRate)
	settings := DefaultSettings()
	shipStart := Point{X: screenWidth / 2, Y: screenHeight / 2}
	return &Game{
		settings:     settings,
		soundManager: NewSoundManager(audioContext),
		playerShip:   NewPlayerShip(shipStart, settings),
		// A decorative field, drifting behind the attract screen
		// until startGame replaces it with wave 1's real field.
		asteroids:   spawnAsteroidField(startingAsteroids, shipStart),
		state:       StateAttract,
		saucerTimer: randomSaucerSpawnDelay(),
	}
}

// spawnAsteroidField creates n Large asteroids of random style at
// random positions, each kept at least asteroidSpawnClearance away
// from avoid (the ship's current position).
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

// Update advances the game by one tick. What actually runs is almost
// entirely delegated to whichever updateX method matches g.state (see
// gamestate.go) -- Update itself only handles the handful of things
// that are the same in every state.
func (g *Game) Update() error {
	// Advances any in-progress loop fade-outs (see StopLoop).
	g.soundManager.Update()
	g.updateDebugKeys()

	dt := 1.0 / float64(ebiten.TPS())

	switch g.state {
	case StateAttract:
		g.updateAttract(dt)
	case StatePlaying:
		g.updatePlaying(dt)
	case StateWaveClear:
		g.updateWaveClear(dt)
	case StatePlayerDying:
		g.updatePlayerDying(dt)
	case StateGameOver:
		g.updateGameOver(dt)
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	for _, a := range g.asteroids {
		a.Draw(screen)
	}
	if g.saucer != nil {
		g.saucer.Draw(screen)
	}
	// No ship on the attract screen -- it hasn't spawned yet.
	if g.state != StateAttract {
		g.playerShip.Draw(screen)
	}
	for _, e := range g.explosions {
		e.Draw(screen)
	}

	switch g.state {
	case StateAttract:
		drawCenteredText(screen, "GLROIDS", 260, titleTextScale, color.White)
		drawCenteredText(screen, "PRESS ENTER OR TAP TO PLAY", 340, promptTextScale, color.White)
	case StatePlaying, StateWaveClear, StatePlayerDying:
		drawLives(screen, g.playerShip, g.lives)
		drawCenteredText(screen, fmt.Sprintf("SCORE %d", g.score), 30, hudTextScale, color.White)
		drawText(screen, fmt.Sprintf("WAVE %d", g.wave), screenWidth-170, 30, hudTextScale, color.White)
	case StateGameOver:
		drawLives(screen, g.playerShip, g.lives)
		drawCenteredText(screen, fmt.Sprintf("SCORE %d", g.score), 30, hudTextScale, color.White)
		drawText(screen, fmt.Sprintf("WAVE %d", g.wave), screenWidth-170, 30, hudTextScale, color.White)
		drawCenteredText(screen, "GAME OVER", 300, titleTextScale, color.White)
		drawCenteredText(screen, "PRESS ENTER OR TAP TO CONTINUE", 380, promptTextScale, color.White)
	}

	// The on-screen buttons only mean anything while ship.Update is
	// actually reading them (see updatePlaying/updateWaveClear) --
	// drawn here rather than folded into the switch above since both
	// those states already have their own case.
	if g.touchControls && (g.state == StatePlaying || g.state == StateWaveClear) {
		drawTouchControls(screen)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// World space is a screenWidth x screenHeight canvas, matching
	// SetWindowSize 1:1 -- one path/position unit is one screen pixel,
	// no implicit stretch factor to account for. Its aspect ratio can
	// change at runtime (see Game.enableTouchControls), so this reads
	// the current values rather than a fixed constant.
	return int(screenWidth), int(screenHeight)
}

func main() {
	ebiten.SetWindowSize(int(screenWidth), int(screenHeight))
	ebiten.SetWindowTitle("glroids")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
