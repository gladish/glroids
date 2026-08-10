package main

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// explosionSparkCount is how many sparks radiate from a single burst.
const explosionSparkCount = 14

// explosionLifetime is how long a burst lives, in seconds, before the
// whole thing expires and gets removed -- short and punchy rather
// than a lingering effect.
const explosionLifetime = 0.35

// explosionSpeedRange bounds how fast each spark flies outward from
// the impact point, in world units/sec.
var explosionSpeedRange = [2]float64{90, 260}

// explosionSparkLength is how long each spark's drawn streak is, in
// world units -- drawn as a short line trailing behind its direction
// of travel rather than a plain dot, for a "sparkle" look consistent
// with the rest of the game's vector-line art.
const explosionSparkLength = 6.0

// explosionStrokeWidth is how thick each spark's streak is drawn.
const explosionStrokeWidth = 2.0

// spark is one radiating fragment of an explosion: a straight-line
// path traveling outward at a fixed velocity from the moment of
// impact. Sparks don't wrap, tumble, or collide with anything -- pure
// decoration, not gameplay state.
type spark struct {
	Pos Point
	Vel Vector
}

// Explosion is a short-lived burst of sparks at a point of impact.
// It's a GameObject in its own right -- Pos marks where the burst
// happened -- even though what actually animates is the independent
// Pos/Vel each spark carries. Satisfies the same Update/Draw shape as
// every other Entity, so Game can hold and drive it the same way it
// drives the ship and rocks.
type Explosion struct {
	GameObject
	sparks   []spark
	age      float64 // seconds since spawn
	lifetime float64
}

// NewExplosion spawns a burst of explosionSparkCount sparks at pos,
// each flying outward in its own random direction at a random speed
// within explosionSpeedRange -- so every hit's burst looks a little
// different rather than a stamped-out animation.
func NewExplosion(pos Point) *Explosion {
	sparks := make([]spark, explosionSparkCount)
	for i := range sparks {
		angle := rand.Float64() * 2 * math.Pi
		speed := explosionSpeedRange[0] + rand.Float64()*(explosionSpeedRange[1]-explosionSpeedRange[0])
		sparks[i] = spark{
			Pos: pos,
			Vel: FromAngle(angle).Scale(speed),
		}
	}
	return &Explosion{
		GameObject: GameObject{Pos: pos},
		sparks:     sparks,
		lifetime:   explosionLifetime,
	}
}

// Update flies every spark outward along its own straight line and
// ages the burst. Sparks don't decelerate or screen-wrap -- Expired()
// is what ends the effect, regardless of where the sparks end up.
func (e *Explosion) Update(dt float64) {
	e.age += dt
	for i := range e.sparks {
		e.sparks[i].Pos = e.sparks[i].Pos.Add(e.sparks[i].Vel.Scale(dt))
	}
}

// Expired reports whether the burst has outlived its lifetime and
// should be dropped from Game.explosions.
func (e *Explosion) Expired() bool {
	return e.age >= e.lifetime
}

// Draw strokes each spark as a short streak trailing behind its
// direction of travel, fading out (via alpha) as the burst ages --
// bright on impact, gone by the time Expired() reports true.
func (e *Explosion) Draw(screen *ebiten.Image) {
	fade := 1 - e.age/e.lifetime
	if fade < 0 {
		fade = 0
	}
	c := color.RGBA{R: 255, G: 255, B: 255, A: uint8(255 * fade)}

	for _, s := range e.sparks {
		tail := s.Pos.Sub(s.Vel.Normalized().Scale(explosionSparkLength))
		vector.StrokeLine(screen,
			float32(tail.X), float32(tail.Y),
			float32(s.Pos.X), float32(s.Pos.Y),
			explosionStrokeWidth, c, true,
		)
	}
}
