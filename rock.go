package main

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
)

// RockStyle selects which of the four hand-adapted outlines an
// asteroid is drawn with, ported from the original arcade ROM's rock
// vector data. Every style's path data below is authored at
// RockScale.Large -- Medium and Small aren't stored separately, they
// are that same path scaled down at spawn time (see rockScaleFactor),
// matching the original game's roughly 2x step down in size between
// large -> medium -> small.
type RockStyle int

const (
	RockStyleOne RockStyle = iota
	RockStyleTwo
	RockStyleThree
	RockStyleFour
	numRockStyles // sentinel: count of styles above, used to pick a random one
)

// RockScale is how big an asteroid is.
type RockScale int

const (
	RockScaleSmall RockScale = iota
	RockScaleMedium
	RockScaleLarge
)

// rockScaleFactor scales a style's Large path (and its radius) down
// to Medium/Small.
var rockScaleFactor = map[RockScale]float64{
	RockScaleLarge:  1.0,
	RockScaleMedium: 0.5,
	RockScaleSmall:  0.25,
}

// rockPaths holds each style's outline in local (unrotated,
// asteroid-centered) coordinates at RockScale.Large, adapted from the
// original arcade ROM's "Rock Pattern 1-4" vector data.
var rockPaths = [numRockStyles][]Point{
	RockStyleOne: {
		{X: 0, Y: 16},
		{X: 16, Y: 32},
		{X: 32, Y: 16},
		{X: 24, Y: 0},
		{X: 32, Y: -16},
		{X: 8, Y: -32},
		{X: -16, Y: -32},
		{X: -32, Y: -16},
		{X: -32, Y: 16},
		{X: -16, Y: 32},
	},
	RockStyleTwo: {
		{X: 16, Y: 8},
		{X: 32, Y: 16},
		{X: 16, Y: 32},
		{X: 0, Y: 24},
		{X: -16, Y: 32},
		{X: -32, Y: 16},
		{X: -24, Y: 0},
		{X: -32, Y: -16},
		{X: -16, Y: -32},
		{X: -8, Y: -24},
		{X: 16, Y: -32},
		{X: 32, Y: -8},
	},
	RockStyleThree: {
		{X: -16, Y: 0},
		{X: -32, Y: -8},
		{X: -16, Y: -32},
		{X: 0, Y: -8},
		{X: 0, Y: -32},
		{X: 16, Y: -32},
		{X: 32, Y: -8},
		{X: 32, Y: 8},
		{X: 16, Y: 32},
		{X: -8, Y: 32},
		{X: -32, Y: 8},
	},
	RockStyleFour: {
		{X: 8, Y: 0},
		{X: 32, Y: 8},
		{X: 32, Y: 16},
		{X: 8, Y: 32},
		{X: -16, Y: 32},
		{X: -8, Y: 16},
		{X: -32, Y: 16},
		{X: -32, Y: -8},
		{X: -16, Y: -32},
		{X: 8, Y: -24},
		{X: 16, Y: -32},
		{X: 32, Y: -16},
	},
}

// scaledRockPath returns style's outline scaled to scale, deriving
// Medium/Small from the stored Large path rather than authoring a
// separate outline per size.
func scaledRockPath(style RockStyle, scale RockScale) []Point {
	src := rockPaths[style]
	factor := rockScaleFactor[scale]
	if factor == 1.0 {
		return src
	}
	out := make([]Point, len(src))
	for i, p := range src {
		out[i] = p.Scale(factor)
	}
	return out
}

// rockBaseRadius is the collision/spawn radius of a Large asteroid --
// half the ~64-unit span of the Large path data above. Medium and
// Small scale it the same way they scale the outline.
const rockBaseRadius = 32.0

// rockSpeedRange gives the min/max drift speed (world units/sec) for
// each size -- small asteroids drift faster than large ones, same as
// the arcade original.
var rockSpeedRange = map[RockScale][2]float64{
	RockScaleLarge:  {20, 40},
	RockScaleMedium: {40, 70},
	RockScaleSmall:  {70, 110},
}

// rockMaxSpin is the fastest an asteroid tumbles, in radians/sec --
// NewAsteroid picks a random spin up to this, in either direction.
const rockMaxSpin = 1.5

// Asteroid is a tumbling rock: a GameObject (Pos/Vel/Rotation/RotVel)
// plus the local-space outline picked by its Style/Scale at spawn time.
type Rock struct {
	GameObject
	Style RockStyle
	Scale RockScale
	Path  []Point

	// WorldPath is Path rotated by Rotation and translated by Pos,
	// recomputed once per Update tick -- see PlayerShip.WorldPath for
	// why this is cached rather than redone in Draw.
	WorldPath []Point
}

// NewAsteroid spawns an asteroid of the given style/scale at pos,
// drifting in a random direction at a speed appropriate for its size
// and tumbling at a random spin.
func NewRock(pos Point, style RockStyle, scale RockScale) *Rock {
	speedRange := rockSpeedRange[scale]
	speed := speedRange[0] + rand.Float64()*(speedRange[1]-speedRange[0])
	dir := FromAngle(rand.Float64() * 2 * math.Pi)
	path := scaledRockPath(style, scale)

	return &Rock{
		GameObject: GameObject{
			Pos:    pos,
			Vel:    dir.Scale(speed),
			RotVel: (rand.Float64()*2 - 1) * rockMaxSpin,
		},
		Style: style,
		Scale: scale,
		Path:  path,
		// Set up front (rather than left nil until the next Update)
		// so a rock spawned mid-tick -- e.g. a split's children --
		// still has a valid outline for this frame's Draw call.
		WorldPath: TransformPath(path, 0, pos),
	}
}

// NewRandomAsteroid spawns a Large asteroid of a random style at pos
// -- the usual way a fresh wave's rocks get created, before any of
// them have split into Medium/Small.
func NewRandomAsteroid(pos Point) *Rock {
	return NewRock(pos, RockStyle(rand.Intn(int(numRockStyles))), RockScaleLarge)
}

// splitSpreadMinAngle/MaxAngle bound how far each child's heading
// diverges from the parent's own direction of travel when a rock
// splits, in radians. This gives the classic "V" of debris kicking
// away from the impact point rather than two clones continuing in a
// single line.
const (
	splitSpreadMinAngle = math.Pi / 12 // 15 degrees
	splitSpreadMaxAngle = math.Pi / 3  // 60 degrees
)

// SplitRock returns the rock(s) left behind when r is destroyed: two
// children one scale down (Large -> Medium, Medium -> Small), spawned
// at r's position and angled off r's own heading in a rough "V" --
// or nil if r was already Small, which the original arcade game (and
// this one) just removes outright.
//
// The 1979 original doesn't conserve momentum on a split -- per
// contemporary breakdowns of its behavior, the two children's
// headings are only loosely related to the parent's, and each moves
// at a speed typical of its (smaller, therefore faster) size rather
// than inheriting the parent's velocity. This mirrors that: NewRock's
// usual size-based random speed for the child's scale, direction
// perturbed off the parent's rather than fully randomized.
func SplitRock(r *Rock) []*Rock {
	if r.Scale == RockScaleSmall {
		return nil
	}
	childScale := r.Scale - 1

	baseAngle := rand.Float64() * 2 * math.Pi
	if r.Vel != (Vector{}) {
		baseAngle = math.Atan2(r.Vel.Y, r.Vel.X)
	}

	children := make([]*Rock, 2)
	for i := range children {
		style := RockStyle(rand.Intn(int(numRockStyles)))
		child := NewRock(r.Pos, style, childScale)

		spread := splitSpreadMinAngle + rand.Float64()*(splitSpreadMaxAngle-splitSpreadMinAngle)
		if i == 1 {
			spread = -spread
		}
		// NewRock already picked a random speed appropriate for
		// childScale -- keep that magnitude, just redirect it.
		child.Vel = FromAngle(baseAngle + spread).Scale(child.Vel.Length())

		children[i] = child
	}
	return children
}

// Radius returns the asteroid's current collision radius, derived
// from its scale the same way its outline is.
func (a *Rock) Radius() float64 {
	return rockBaseRadius * rockScaleFactor[a.Scale]
}

// Update tumbles and drifts the asteroid, then wraps it around the
// screen edges same as the player ship.
func (a *Rock) Update(dt float64) {
	a.Integrate(dt)
	a.WrapPosition(screenWidth, screenHeight)
	a.WorldPath = TransformPath(a.Path, a.Rotation, a.Pos)
}

func (a *Rock) Draw(screen *ebiten.Image) {
	strokeClosedPath(screen, a.WorldPath)
}
