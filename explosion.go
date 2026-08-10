package main

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// explosionSparkLength is how long each spark's drawn streak is, in
// world units -- drawn as a short line trailing behind its direction
// of travel rather than a plain dot, for a "sparkle" look consistent
// with the rest of the game's vector-line art.
const explosionSparkLength = 6.0

// explosionStrokeWidth is how thick each spark's streak is drawn.
const explosionStrokeWidth = 2.0

// burstConfig is the tunable shape of a spark burst: how many sparks,
// how fast they fly, and how long the whole thing lasts. NewExplosion
// and NewShipExplosion are both just newBurst with different numbers
// -- a rock pop and the player's own death are the same kind of
// effect, scaled up for the bigger, more dramatic event.
type burstConfig struct {
	sparkCount         int
	speedMin, speedMax float64
	lifetime           float64
}

// rockHitBurst is a small, quick pop -- the effect for an ordinary
// rock getting shot.
var rockHitBurst = burstConfig{
	sparkCount: 14,
	speedMin:   90,
	speedMax:   260,
	lifetime:   0.35,
}

// shipDeathBurst is bigger and lasts longer than a rock pop, since
// losing the ship is a much bigger moment than popping a rock.
var shipDeathBurst = burstConfig{
	sparkCount: 26,
	speedMin:   60,
	speedMax:   240,
	lifetime:   0.6,
}

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

// NewExplosion spawns the standard rock-hit spark burst at pos, each
// spark flying outward in its own random direction/speed so every
// hit's burst looks a little different rather than a stamped-out
// animation.
func NewExplosion(pos Point) *Explosion {
	return newBurst(pos, rockHitBurst)
}

// NewShipExplosion spawns the bigger, longer-lived burst used when
// the player's ship is destroyed -- same effect as NewExplosion,
// scaled up via shipDeathBurst rather than a separate animation.
func NewShipExplosion(pos Point) *Explosion {
	return newBurst(pos, shipDeathBurst)
}

// newBurst builds an Explosion of cfg's shape at pos.
func newBurst(pos Point, cfg burstConfig) *Explosion {
	sparks := make([]spark, cfg.sparkCount)
	for i := range sparks {
		angle := rand.Float64() * 2 * math.Pi
		speed := cfg.speedMin + rand.Float64()*(cfg.speedMax-cfg.speedMin)
		sparks[i] = spark{
			Pos: pos,
			Vel: FromAngle(angle).Scale(speed),
		}
	}
	return &Explosion{
		GameObject: GameObject{Pos: pos},
		sparks:     sparks,
		lifetime:   cfg.lifetime,
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
