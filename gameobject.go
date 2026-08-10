package main

import "github.com/hajimehoshi/ebiten/v2"

// Entity is anything the game loop can update and draw, regardless of
// what it embeds. PlayerShip, Asteroid, Bullet, Saucer, etc. all
// satisfy this by embedding GameObject and adding their own
// Update/Draw -- it's what lets a single []Entity hold every kind of
// object in the game.
type Entity interface {
	Update(dt float64)
	Draw(screen *ebiten.Image)
}

// GameObject holds the state and physics step shared by every entity:
// position, velocity, rotation, and rotational velocity. It's not a
// base class in the C++ sense -- concrete types embed it for its
// fields and call Integrate explicitly from their own Update, since
// Go has no virtual dispatch to call it automatically.
type GameObject struct {
	Pos      Point
	Vel      Vector
	Rotation float64 // radians
	RotVel   float64 // radians/sec
}

// Integrate advances position and rotation by one physics step of dt
// seconds. Call it from a concrete type's Update, after handling any
// input or behavior that might change Vel or RotVel for this tick.
func (o *GameObject) Integrate(dt float64) {
	o.Pos = o.Pos.Add(o.Vel.Scale(dt))
	o.Rotation += o.RotVel * dt
}

// wrapBufferPct is how far past the edge (as a fraction of screen
// width/height) an object must fully travel before it wraps to the
// opposite side. Without it, a wrap at exactly the edge would pop the
// object from "half visible on the right" straight to "half visible
// on the left" in one frame; the buffer lets it clear the screen
// first so the wrap happens out of sight.
const wrapBufferPct = 0.04

// WrapPosition wraps o.Pos to the opposite edge of a w x h screen
// once it drifts wrapBufferPct past that edge, so ships/asteroids/etc.
// that fly off one side smoothly reappear on the other.
func (o *GameObject) WrapPosition(w, h float64) {
	bufX := w * wrapBufferPct
	bufY := h * wrapBufferPct

	switch {
	case o.Pos.X > w+bufX:
		o.Pos.X = -bufX
	case o.Pos.X < -bufX:
		o.Pos.X = w + bufX
	}

	switch {
	case o.Pos.Y > h+bufY:
		o.Pos.Y = -bufY
	case o.Pos.Y < -bufY:
		o.Pos.Y = h + bufY
	}
}
