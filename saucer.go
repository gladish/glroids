package main

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
)

// SaucerKind is which of the two flying-saucer enemies a Saucer is.
// Their movement/spawn/firing policy was consulted from the original
// arcade's public disassembly and RAM map --
// https://6502disassembly.com/va-asteroids/ and
// http://computerarcheology.com/Arcade/Asteroids/RAMUse.html -- rather
// than guessed at: the two kinds share a shape and the same basic
// flight pattern, but differ in size, point value, and how (well)
// they aim (see Game.saucerAimDirection in gamestate.go for the
// aiming policy itself).
type SaucerKind int

const (
	SaucerLarge SaucerKind = iota
	SaucerSmall
)

// saucerScore is how many points destroying each kind of saucer is
// worth -- the original arcade's actual values (200/990), not the
// commonly-misremembered round 1000 for the small one.
var saucerScore = map[SaucerKind]int{
	SaucerLarge: 200,
	SaucerSmall: 990,
}

// saucerScale shrinks the small saucer's outline/radius relative to
// the large one -- like Rock, one shape is authored at full (Large)
// size and scaled down rather than a second outline drawn from
// scratch.
var saucerScale = map[SaucerKind]float64{
	SaucerLarge: 1.0,
	SaucerSmall: 0.5,
}

// saucerSpeed is how fast each kind drifts horizontally, in world
// units/sec -- the small saucer is peppier, same size-vs-speed
// relationship Rock uses for its own scales.
var saucerSpeed = map[SaucerKind]float64{
	SaucerLarge: 90.0,
	SaucerSmall: 130.0,
}

// saucerHullPath/saucerDomePath are the saucer's outline in local
// (unrotated -- saucers never rotate, unlike the ship or rocks --
// saucer-centered) coordinates, authored at SaucerLarge's scale and
// drawn as two separate closed loops (same two-Path approach
// PlayerShip takes with its body and thrust flame) so the silhouette
// actually reads as a saucer instead of a plain hexagon.
var saucerHullPath = []Point{
	{X: -20, Y: 0},
	{X: -10, Y: -6},
	{X: 10, Y: -6},
	{X: 20, Y: 0},
	{X: 10, Y: 6},
	{X: -10, Y: 6},
}

var saucerDomePath = []Point{
	{X: -8, Y: -6},
	{X: -5, Y: -14},
	{X: 5, Y: -14},
	{X: 8, Y: -6},
}

// saucerBaseRadius is the collision radius of a Large saucer -- half
// the hull's ~40-unit width, same "half the widest extent" approach
// shipRadius and rockBaseRadius take.
const saucerBaseRadius = 20.0

// saucerEdgeMargin is how far off-screen (horizontally) a saucer
// spawns, and how far past the far edge it travels before Escaped
// reports true -- keeps its entrance and exit off-screen instead of
// popping in/out right at the visible edge.
const saucerEdgeMargin = 40.0

// jinkIntervalMin/Max bound how often (seconds) a saucer picks a new
// vertical drift, giving it the classic saucer zig-zag flight instead
// of a dead-straight line across the screen.
const jinkIntervalMin = 0.4
const jinkIntervalMax = 1.1

// jinkVerticalFrac is how fast a saucer's vertical drift can be, as a
// fraction of its horizontal speed.
const jinkVerticalFrac = 0.6

// maxActiveSaucerShots caps how many of the saucer's own shots can be
// in flight at once -- the original's RAM map reserves storage for
// exactly two saucer shots at a time, so this mirrors that rather
// than picking an arbitrary number.
const maxActiveSaucerShots = 2

// saucerShotInterval bounds the random gap (seconds) between one
// saucer shot and the next, per kind.
var saucerShotInterval = map[SaucerKind][2]float64{
	SaucerLarge: {1.0, 2.0},
	SaucerSmall: {0.8, 1.6},
}

// saucerFirstShotDelay is how long a freshly spawned saucer waits
// before its first shot, giving the player a beat to notice it.
const saucerFirstShotDelay = 1.0

// Saucer is an enemy flying saucer: a GameObject (for Pos/Vel) plus
// the local-space outline picked by its Kind, and its own small pool
// of shots. Unlike Rock/PlayerShip it never rotates -- Rotation stays
// 0 for its whole lifetime -- matching the original, where saucers
// are always drawn upright.
type Saucer struct {
	GameObject
	Kind     SaucerKind
	Path     []Point
	DomePath []Point

	// WorldPath/WorldDomePath are Path/DomePath translated by Pos,
	// recomputed once per Update tick -- see PlayerShip.WorldPath for
	// why this is cached rather than redone in Draw.
	WorldPath     []Point
	WorldDomePath []Point

	Shots []*Shot

	// ReadyToFire is true on any tick shotTimer has run out --
	// Game.updateSaucer reads this to pick a direction (it knows
	// where the player is; Saucer doesn't) and calls fire, which
	// clears the flag and restarts the timer. Same
	// timer-elapsed-this-tick flag pattern as PlayerShip.Fired, just
	// driven by an internal timer instead of a keypress.
	ReadyToFire bool
	shotTimer   float64 // seconds until ReadyToFire goes true

	jinkTimer float64 // seconds until the next vertical drift change
}

// scalePoints returns pts scaled by factor about the local origin --
// same helper scaledRockPath provides for Rock, generalized since
// Saucer scales two separate paths (hull and dome) rather than one.
func scalePoints(pts []Point, factor float64) []Point {
	if factor == 1.0 {
		return pts
	}
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[i] = p.Scale(factor)
	}
	return out
}

// NewSaucer spawns a saucer of the given kind just off whichever
// horizontal edge is farther from avoid (typically the player's
// current position, so it doesn't appear right on top of the ship),
// heading straight across at a random height and that kind's speed.
func NewSaucer(kind SaucerKind, avoid Point) *Saucer {
	scale := saucerScale[kind]
	hull := scalePoints(saucerHullPath, scale)
	dome := scalePoints(saucerDomePath, scale)

	fromLeft := avoid.X > screenWidth/2
	startX := screenWidth + saucerEdgeMargin
	dir := -1.0
	if fromLeft {
		startX = -saucerEdgeMargin
		dir = 1.0
	}

	s := &Saucer{
		GameObject: GameObject{
			Pos: Point{X: startX, Y: rand.Float64() * screenHeight},
			Vel: Vector{X: dir * saucerSpeed[kind], Y: 0},
		},
		Kind:      kind,
		Path:      hull,
		DomePath:  dome,
		Shots:     make([]*Shot, 0, maxActiveSaucerShots),
		shotTimer: saucerFirstShotDelay,
	}
	s.pickNewJink()

	// Set up front (rather than left nil until the next Update), same
	// reasoning as Rock.WorldPath/NewPlayerShip's WorldPath -- Draw
	// can run before this saucer's first Update tick.
	s.WorldPath = TransformPath(s.Path, 0, s.Pos)
	s.WorldDomePath = TransformPath(s.DomePath, 0, s.Pos)
	return s
}

// Radius returns the saucer's current collision radius, derived from
// its kind the same way its outline is.
func (s *Saucer) Radius() float64 {
	return saucerBaseRadius * saucerScale[s.Kind]
}

// pickNewJink rolls a new vertical drift (see jinkVerticalFrac) and
// resets the countdown to the next one (see jinkIntervalMin/Max).
func (s *Saucer) pickNewJink() {
	speed := math.Abs(s.Vel.X)
	s.Vel.Y = (rand.Float64()*2 - 1) * jinkVerticalFrac * speed
	s.jinkTimer = jinkIntervalMin + rand.Float64()*(jinkIntervalMax-jinkIntervalMin)
}

// Escaped reports whether the saucer has flown past the far edge of
// the screen. It never wraps horizontally (see Update), so once it's
// past the far edge it's gone for good -- Game.updateSaucer despawns
// it with no score awarded, same as the original arcade just letting
// an un-destroyed saucer fly off.
func (s *Saucer) Escaped() bool {
	return s.Pos.X < -saucerEdgeMargin || s.Pos.X > screenWidth+saucerEdgeMargin
}

// Update drifts the saucer (horizontal cruise plus vertical jink),
// ages its shot timer and in-flight shots, and refreshes its
// world-space outline. It does not decide to fire -- that needs to
// know where the player is, which only Game knows (see
// Game.updateSaucer) -- Update just tracks whether the timer has run
// out (see ReadyToFire).
func (s *Saucer) Update(dt float64) {
	s.Integrate(dt)

	// Only the vertical axis wraps -- horizontally the saucer flies a
	// single traversal across the screen and is meant to exit off the
	// far edge (see Escaped), not reappear on the near one the way
	// wrapping both axes (like Rock/PlayerShip do) would cause.
	bufY := screenHeight * wrapBufferPct
	switch {
	case s.Pos.Y > screenHeight+bufY:
		s.Pos.Y = -bufY
	case s.Pos.Y < -bufY:
		s.Pos.Y = screenHeight + bufY
	}

	s.jinkTimer -= dt
	if s.jinkTimer <= 0 {
		s.pickNewJink()
	}

	s.shotTimer -= dt
	if s.shotTimer <= 0 {
		s.ReadyToFire = true
	}

	// Same compact-in-place pattern as PlayerShip.updateBullets.
	live := s.Shots[:0]
	for _, b := range s.Shots {
		b.Update(dt)
		if !b.Expired() {
			live = append(live, b)
		}
	}
	s.Shots = live

	s.WorldPath = TransformPath(s.Path, 0, s.Pos)
	s.WorldDomePath = TransformPath(s.DomePath, 0, s.Pos)
}

// fire spawns a new shot heading in dir (Game picks dir -- see
// saucerAimDirection in gamestate.go) and restarts the timer until
// the next one. If the saucer is already at maxActiveSaucerShots this
// still restarts the timer (so it doesn't immediately retry next
// tick) but doesn't spawn a shot -- in practice this rarely comes up,
// since saucerShotInterval's minimums already run longer than a
// shot's own flight time.
func (s *Saucer) fire(dir Vector) {
	s.ReadyToFire = false
	interval := saucerShotInterval[s.Kind]
	s.shotTimer = interval[0] + rand.Float64()*(interval[1]-interval[0])
	if len(s.Shots) >= maxActiveSaucerShots {
		return
	}
	s.Shots = append(s.Shots, NewShot(s.Pos, dir))
}

// Draw strokes the saucer's hull and dome, plus any shots it has in
// flight, using the world-space points Update already computed.
func (s *Saucer) Draw(screen *ebiten.Image) {
	strokeClosedPath(screen, s.WorldPath)
	strokeClosedPath(screen, s.WorldDomePath)
	for _, b := range s.Shots {
		b.Draw(screen)
	}
}
