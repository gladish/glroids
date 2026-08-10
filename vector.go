package main

import "math"

type Vector struct {
	X float64
	Y float64
}

func (v Vector) Add(o Vector) Vector    { return Vector{v.X + o.X, v.Y + o.Y} }
func (v Vector) Sub(o Vector) Vector    { return Vector{v.X - o.X, v.Y - o.Y} }
func (v Vector) Scale(s float64) Vector { return Vector{v.X * s, v.Y * s} }
func (v Vector) Length() float64        { return math.Hypot(v.X, v.Y) }

func (v Vector) Normalized() Vector {
	l := v.Length()
	if l == 0 {
		return Vector{}
	}
	return v.Scale(1 / l)
}

// Rotated returns v rotated by rad radians about the origin. Note
// that in screen space (Y grows downward), a positive rad rotates
// clockwise as drawn, not counter-clockwise -- e.g. Vector2{X: 1}
// rotated by +90° lands at Vector2{Y: 1}, which points down.
func (v Vector) Rotated(rad float64) Vector {
	sin, cos := math.Sincos(rad)
	return Vector{
		X: v.X*cos - v.Y*sin,
		Y: v.X*sin + v.Y*cos,
	}
}

// FromAngle returns a unit vector pointing at the given angle (radians),
// handy for turning a ship's Rotation into a thrust direction.
func FromAngle(rad float64) Vector {
	sin, cos := math.Sincos(rad)
	return Vector{X: cos, Y: sin}
}

type Point = Vector

// TransformPath rotates each point in local (by rad radians about the
// origin) and translates the result by pos, returning the resulting
// world-space points. This is how a local, ship/asteroid-centered
// outline becomes the actual polygon to draw -- and, later, the
// polygon to run collision checks against. Compute it once per tick
// (after an entity's Rotation/Pos settle for that tick) and cache the
// result rather than re-deriving it inside Draw every frame.
func TransformPath(local []Point, rad float64, pos Point) []Point {
	world := make([]Point, len(local))
	for i, v := range local {
		world[i] = v.Rotated(rad).Add(pos)
	}
	return world
}
