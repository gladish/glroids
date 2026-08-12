package main

import (
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// shipForwardLocal is the ship's forward (nose) direction in its own
// local space -- it points "up" (negative Y) when Rotation == 0,
// matching shipPath's nose at {0, -14}.
var shipForwardLocal = Vector{X: 0, Y: -1}

// shipTurnSpeed is how fast the ship rotates while a turn key is
// held, in radians/sec.
const shipTurnSpeed = 4.0

// shipThrustAccel is how fast the ship gains speed while thrusting,
// in world units/sec^2. Space is frictionless -- releasing Up stops
// the acceleration, not the ship, so it keeps coasting at whatever
// velocity it had.
const shipThrustAccel = 300.0

// thrustFlashTicks is how many game ticks the flame stays visible (or
// hidden) before toggling, giving the classic flickering-flame look
// instead of a steady one. Lower = faster flicker.
const thrustFlashTicks = 5

// hyperspaceDeathChance is the odds a hyperspace jump goes wrong and
// destroys the ship on reappear -- the classic Asteroids gamble: it's
// an escape hatch, not a guaranteed-safe one.
const hyperspaceDeathChance = 0.167

// respawnInvulnDuration is how long a freshly respawned ship is safe
// from rock collisions -- long enough to get clear of whatever's
// sitting on the spawn point before it's vulnerable again.
const respawnInvulnDuration = 2 * time.Second

// invulnBlinkTicks is how many ticks the ship stays visible (or
// hidden) between toggles while invulnerable -- same flicker trick as
// thrustFlashTicks, just its own tempo.
const invulnBlinkTicks = 6

// shipRadius approximates the ship's outline as a circle for
// collision purposes, same approach Rock.Radius takes for its own
// (jagged, rotating) shape. shipPath's farthest point from center is
// the wingtips at ~17 units out; this is pulled in from that so a
// rock has to actually reach the hull rather than just clip the
// corner of the ship's empty bounding circle.
const shipRadius = 12.0

// PlayerShip is the player's ship: a GameObject (for Pos/Vel/Rotation)
// plus the local-space outline drawn at its current position/heading.
type PlayerShip struct {
	GameObject
	Path       []Point
	ThrustPath []Point

	// WorldPath/WorldThrustPath are Path/ThrustPath rotated by
	// Rotation and translated by Pos, recomputed once per Update tick
	// (see TransformPath). Draw just strokes these directly, and this
	// is also what collision detection should read later -- one
	// transform per tick, shared by both consumers instead of each
	// re-deriving it.
	WorldPath       []Point
	WorldThrustPath []Point

	Thrusting   bool
	thrustTicks int // ticks elapsed while thrusting, drives the flame flicker

	Shots []*Shot // currently in-flight shots, capped at maxActiveBullets
	Fired bool    // true only on the tick a new bullet was just spawned

	Hyperspaced         bool // true only on the tick a hyperspace jump was initiated
	HyperspaceDestroyed bool // true only on the tick a hyperspace return proved fatal

	// Alive is false while the ship is destroyed and waiting on Game
	// to respawn it (see Game.killPlayer/updatePlayerDying) -- Draw
	// skips a dead ship's body/flame the same way it already skips a
	// hidden (mid-hyperspace) one.
	Alive bool

	Keys                  KeyBindings   // which keyboard keys drive turning/thrust/fire
	RespawnHiddenDuration time.Duration // how long the ship is hidden/in-transit during a hyperspace jump

	// hiddenSecondsLeft counts down from RespawnHiddenDuration.Seconds()
	// once a jump starts; the ship is hidden (see Hidden) while it's
	// above zero. Tracked in seconds, like dt, rather than as a
	// time.Duration, so Update can just subtract dt from it each tick.
	hiddenSecondsLeft float64

	// hyperspacePending is true from the moment a jump is initiated
	// until hiddenSecondsLeft reaches zero, at which point Update
	// resolves the jump (see resolveHyperspaceReturn) -- the fate
	// roll happens on return, not on initiation, so a jump reads as
	// "vanish, travel, then arrive safely or not" rather than being
	// decided before the ship even leaves.
	hyperspacePending bool

	// invulnSecondsLeft counts down from respawnInvulnDuration once a
	// fresh respawn grants it (see grantInvulnerability); the ship is
	// safe from rock collisions (see Invulnerable) and flickers (see
	// blinkVisible) while it's above zero. Same seconds-not-Duration
	// reasoning as hiddenSecondsLeft.
	invulnSecondsLeft float64
	invulnTicks       int // ticks elapsed while invulnerable, drives the flicker
}

func NewPlayerShip(pos Point, settings Settings) *PlayerShip {

	// shipPath is the ship's outline in local (unrotated, ship-centered)
	// coordinates. Rotation == 0 means the nose points straight up, and
	// the points are stroked as a closed loop in this order.
	var shipPath = []Vector{
		{X: 0, Y: -14},
		{X: 11, Y: 13},
		{X: 7, Y: 10},
		{X: -7, Y: 10},
		{X: -11, Y: 13},
	}

	// thrustFlamePath is the little flame drawn behind the ship while
	// thrusting, in the same local space as shipPath.
	var thrustFlamePath = []Vector{
		{X: 5, Y: 12},
		{X: -5, Y: 12},
		{X: 0, Y: 24},
	}

	return &PlayerShip{
		GameObject:            GameObject{Pos: pos},
		Path:                  shipPath,
		ThrustPath:            thrustFlamePath,
		Alive:                 true,
		Shots:                 make([]*Shot, 0, maxActiveShots),
		Keys:                  settings.Keys,
		RespawnHiddenDuration: settings.RespawnHiddenDuration,
		// Set up front (rather than left nil until the next Update),
		// same reasoning as Rock.WorldPath -- Draw can run before this
		// ship's first Update tick (e.g. the frame right after
		// Game.startGame flips state to Playing but before this
		// ship's own Update has run), and an empty WorldPath crashes
		// strokeClosedPath.
		WorldPath:       TransformPath(shipPath, 0, pos),
		WorldThrustPath: TransformPath(thrustFlamePath, 0, pos),
	}
}

// Forward returns the ship's current nose direction as a unit vector,
// derived by rotating its local forward vector the same way the
// outline is rotated for drawing -- so thrust always pushes exactly
// where the ship is drawn as pointing.
func (p *PlayerShip) Forward() Vector {
	return shipForwardLocal.Rotated(p.Rotation)
}

// Radius returns the ship's collision radius (see shipRadius).
func (p *PlayerShip) Radius() float64 {
	return shipRadius
}

// Update turns the ship while Left/Right is held (stopping the
// instant it's released), accelerates forward while Up is held, and
// integrates the resulting motion. touch carries this tick's
// on-screen-button state (see TouchInput) -- its zero value is "no
// touch input," so this reads identically to before on a device with
// no touchscreen.
func (p *PlayerShip) Update(dt float64, touch TouchInput) {
	p.Fired = false
	p.Hyperspaced = false
	p.HyperspaceDestroyed = false

	switch {
	case ebiten.IsKeyPressed(p.Keys.TurnLeft) || touch.TurnLeft:
		p.RotVel = -shipTurnSpeed // counter-clockwise
	case ebiten.IsKeyPressed(p.Keys.TurnRight) || touch.TurnRight:
		p.RotVel = shipTurnSpeed // clockwise
	default:
		p.RotVel = 0
	}

	p.Thrusting = ebiten.IsKeyPressed(p.Keys.Thrust) || touch.Thrust
	if p.Thrusting {
		p.Vel = p.Vel.Add(p.Forward().Scale(shipThrustAccel * dt))
		p.thrustTicks++
	} else {
		p.thrustTicks = 0 // next thrust burst always starts flame-visible
	}

	p.Integrate(dt)
	p.WrapPosition(screenWidth, screenHeight)

	if p.hiddenSecondsLeft > 0 {
		p.hiddenSecondsLeft -= dt
		if p.hiddenSecondsLeft <= 0 {
			p.hiddenSecondsLeft = 0
			if p.hyperspacePending {
				p.hyperspacePending = false
				p.resolveHyperspaceReturn()
			}
		}
	}

	if p.invulnSecondsLeft > 0 {
		p.invulnSecondsLeft -= dt
		if p.invulnSecondsLeft < 0 {
			p.invulnSecondsLeft = 0
		}
		p.invulnTicks++
	}

	// Rotate/translate once per tick, after Pos and Rotation are
	// final for this tick, so Draw (and later collision checks) just
	// read the result instead of redoing this math.
	p.WorldPath = TransformPath(p.Path, p.Rotation, p.Pos)
	p.WorldThrustPath = TransformPath(p.ThrustPath, p.Rotation, p.Pos)

	p.updateBullets(dt)

	if (inpututil.IsKeyJustPressed(p.Keys.Fire) || touch.Fire) && len(p.Shots) < maxActiveShots {
		// WorldPath[0] is the nose vertex (shipPath[0], {0, -14})
		// already transformed to world space for this tick.
		p.Shots = append(p.Shots, NewShot(p.WorldPath[0], p.Forward()))
		p.Fired = true
	}

	if !p.hyperspacePending && (inpututil.IsKeyJustPressed(p.Keys.Hyperspace) || touch.Hyperspace) {
		p.startHyperspaceJump()
	}
}

// startHyperspaceJump kicks off a jump: the ship vanishes immediately
// and stays hidden/in-transit for RespawnHiddenDuration (see Hidden).
// Where it ends up -- and whether it survives the trip -- isn't
// decided until the jump resolves (see resolveHyperspaceReturn), once
// that window elapses. Ignored while a jump is already pending, so
// mashing the key mid-jump doesn't restart the timer.
func (p *PlayerShip) startHyperspaceJump() {
	p.Hyperspaced = true
	p.hyperspacePending = true
	p.hiddenSecondsLeft = p.RespawnHiddenDuration.Seconds()
}

// resolveHyperspaceReturn rolls hyperspaceDeathChance and either
// relocates the ship to a random point on screen (keeping its current
// velocity, so momentum carries through the jump) or, on a hit, flags
// HyperspaceDestroyed and leaves the ship where it vanished. This
// method doesn't reset the ship itself on a fatal roll -- Game picks
// the flag up (see Game.checkHyperspaceDeath) and routes it through
// the same death/respawn handling as a rock collision. Called by
// Update once hiddenSecondsLeft reaches zero -- the fate of a jump is
// decided on arrival, not on departure.
func (p *PlayerShip) resolveHyperspaceReturn() {
	if rand.Float64() < hyperspaceDeathChance {
		p.HyperspaceDestroyed = true
		return
	}

	p.Pos = Point{X: rand.Float64() * screenWidth, Y: rand.Float64() * screenHeight}
}

// respawn resets the ship to screen center, at rest and pointed
// straight up. Called by Game once it's decided the ship gets to come
// back -- after the death pause following a rock collision or a fatal
// hyperspace jump (see Game.updatePlayerDying), or at the start of a
// fresh game (see Game.startGame).
func (p *PlayerShip) respawn() {
	p.Pos = Point{X: screenWidth / 2, Y: screenHeight / 2}
	p.Vel = Vector{}
	p.Rotation = 0
	p.RotVel = 0

	// Recomputed immediately rather than left for the next Update
	// tick -- Game flips state (and thus what Draw shows) to Playing
	// in the same tick respawn is called from, before this ship's own
	// Update runs again, so Draw needs an up-to-date WorldPath at the
	// new Pos right away (see NewPlayerShip for the same reasoning).
	p.WorldPath = TransformPath(p.Path, p.Rotation, p.Pos)
	p.WorldThrustPath = TransformPath(p.ThrustPath, p.Rotation, p.Pos)
}

// grantInvulnerability starts (or restarts) the ship's post-respawn
// safe window -- see invulnSecondsLeft.
func (p *PlayerShip) grantInvulnerability() {
	p.invulnSecondsLeft = respawnInvulnDuration.Seconds()
	p.invulnTicks = 0
}

// Invulnerable reports whether the ship is currently safe from rock
// collisions after a fresh respawn (see grantInvulnerability).
func (p *PlayerShip) Invulnerable() bool {
	return p.invulnSecondsLeft > 0
}

// Hidden reports whether the ship is mid-hyperspace-jump (in transit,
// fate not yet resolved) and shouldn't be drawn.
func (p *PlayerShip) Hidden() bool {
	return p.hiddenSecondsLeft > 0
}

// updateBullets advances every in-flight bullet and drops any that
// have exceeded their range, compacting the slice in place.
func (p *PlayerShip) updateBullets(dt float64) {
	live := p.Shots[:0]
	for _, b := range p.Shots {
		b.Update(dt)
		if !b.Expired() {
			live = append(live, b)
		}
	}
	p.Shots = live
}

// flameVisible reports whether the thrust flame should be drawn this
// tick -- it toggles on/off every thrustFlashTicks ticks to flicker
// rather than render as a steady, static flame.
func (p *PlayerShip) flameVisible() bool {
	return p.Thrusting && (p.thrustTicks/thrustFlashTicks)%2 == 0
}

// blinkVisible reports whether the ship's body/flame should be drawn
// this tick -- always true unless Invulnerable(), in which case it
// flickers the same way the thrust flame does (see flameVisible), so
// a freshly respawned ship visibly reads as "still safe" until the
// window ends.
func (p *PlayerShip) blinkVisible() bool {
	if !p.Invulnerable() {
		return true
	}
	return (p.invulnTicks/invulnBlinkTicks)%2 == 0
}

// Draw strokes the ship's outline, plus the thrust flame while it's
// flickering on, using the world-space points Update already
// computed. A dead ship (mid-death-pause, see Alive) or one in
// hyperspace transit (see Hidden) draws no body at all; an
// invulnerable one flickers (see blinkVisible). Shots draw regardless
// -- they're independent of whatever state the ship itself is in.
func (p *PlayerShip) Draw(screen *ebiten.Image) {
	if p.Alive && !p.Hidden() && p.blinkVisible() {
		strokeClosedPath(screen, p.WorldPath)
		if p.flameVisible() {
			strokeClosedPath(screen, p.WorldThrustPath)
		}
	}
	for _, b := range p.Shots {
		b.Draw(screen)
	}
}

// strokeClosedPath strokes a set of already-world-space points as a
// closed loop. Shared by every vector-graphics entity (ship body,
// thrust flame, and eventually asteroids/saucers), so they all render
// with the same look.
func strokeClosedPath(screen *ebiten.Image, world []Point) {
	var path vector.Path

	path.MoveTo(float32(world[0].X), float32(world[0].Y))
	for _, w := range world[1:] {
		path.LineTo(float32(w.X), float32(w.Y))
	}
	path.Close()

	vector.StrokePath(screen, &path,
		&vector.StrokeOptions{Width: 2, LineJoin: vector.LineJoinRound},
		&vector.DrawPathOptions{AntiAlias: true},
	)
}
