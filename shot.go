package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// maxActiveShots caps how many of the player's shots can be on
// screen at once -- the classic Asteroids limit that stops you from
// just holding down fire for a solid stream of shots.
const maxActiveShots = 3

// shotSpeed is how fast a fired shot travels, in world units/sec.
const shotSpeed = 700.0

// shotMaxDistance is how far a shot travels before it disappears,
// in world units. Kept well under the screen size so shots have a
// real range limit rather than being able to cross the whole field.
const shotMaxDistance = 500.0

// bulletRadius is the drawn (and, later, collision) radius of a shot.
const shotRadius = 2.0

// Bullet is a small ball fired from the ship's nose that travels in a
// straight line at a constant velocity and disappears once it has
// traveled shotMaxDistance.
type Shot struct {
	GameObject
	traveled float64
}

// NewShow spawns a bullet at pos moving at shotSpeed in direction
// dir, which is expected to be a unit vector (e.g. PlayerShip.Forward).
func NewShot(pos Point, dir Vector) *Shot {
	return &Shot{
		GameObject: GameObject{
			Pos: pos,
			Vel: dir.Scale(shotSpeed),
		},
	}
}

// Update advances the bullet and tracks distance traveled. Bullets
// don't screen-wrap -- once they've gone bulletMaxDistance, they're
// done, wherever they happen to be.
func (shot *Shot) Update(dt float64) {
	shot.Integrate(dt)
	shot.traveled += shotSpeed * dt
}

// Expired reports whether the bullet has traveled its max range and
// should be removed.
func (shot *Shot) Expired() bool {
	return shot.traveled >= shotMaxDistance
}

func (shot *Shot) Draw(screen *ebiten.Image) {
	vector.FillCircle(screen, float32(shot.Pos.X), float32(shot.Pos.Y), shotRadius, color.White, true)
}
