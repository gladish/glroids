package main

// rockScore awards points for destroying a rock, matching the arcade
// original's inverse relationship between size and points -- smaller,
// faster, harder-to-hit rocks are worth more.
var rockScore = map[RockScale]int{
	RockScaleLarge:  20,
	RockScaleMedium: 50,
	RockScaleSmall:  100,
}

// CheckShotRockCollisions tests every in-flight player shot against
// every rock for a circle-circle overlap. On a hit: the shot is
// consumed, the rock is destroyed and replaced by SplitRock's
// children (nothing, if it was already Small), the size-matched bang
// sound plays -- same sounds the debug keys in main.go trigger
// manually -- a spark burst spawns at the shot's position (the point
// of impact), and the rock's size-matched value (see rockScore) is
// added to the score returned. A shot stops at its first hit rather
// than piercing through to a second rock the same tick; separate
// shots can each still score their own hit in one tick.
//
// Returns the surviving shots, the rock field with destroyed rocks
// removed and any split children appended, any new Explosions spawned
// this call, and the total score earned -- the caller owns adding
// those to whatever slice/counter Game.Update/Draw drives.
func CheckShotRockCollisions(shots []*Shot, rocks []*Rock, sm *SoundManager) ([]*Shot, []*Rock, []*Explosion, int) {
	survivingShots := shots[:0]
	var newExplosions []*Explosion
	var scoreGained int

shotLoop:
	for _, s := range shots {
		for i, r := range rocks {
			if !circlesOverlap(s.Pos, shotRadius, r.Pos, r.Radius()) {
				continue
			}

			sm.Play(bangSFXFor(r.Scale))
			newExplosions = append(newExplosions, NewExplosion(s.Pos))
			scoreGained += rockScore[r.Scale]

			// Swap-remove the hit rock, then append whatever it split
			// into (nothing, for a Small rock).
			rocks[i] = rocks[len(rocks)-1]
			rocks = rocks[:len(rocks)-1]
			rocks = append(rocks, SplitRock(r)...)

			continue shotLoop // this shot is spent; don't test it against the rest
		}
		survivingShots = append(survivingShots, s)
	}

	return survivingShots, rocks, newExplosions, scoreGained
}

// CheckShipRockCollision reports whether the player's ship overlaps
// any live rock. The ship is only vulnerable while visible and out of
// its post-respawn grace window -- Hidden() covers the brief
// mid-hyperspace-jump window, which already has its own fate roll
// (see PlayerShip.resolveHyperspaceReturn) and shouldn't also get
// picked off by a rock while it's away, and Invulnerable() covers the
// window right after a respawn where a rock might be sitting on the
// spawn point.
//
// This only reports the hit -- it doesn't touch rocks (the rock that
// hit the ship is left alone) or the ship's Shots. Whatever calls
// this owns deciding what happens to the ship (see Game.killPlayer).
func CheckShipRockCollision(ship *PlayerShip, rocks []*Rock) bool {
	if ship.Hidden() || ship.Invulnerable() {
		return false
	}
	for _, r := range rocks {
		if circlesOverlap(ship.Pos, ship.Radius(), r.Pos, r.Radius()) {
			return true
		}
	}
	return false
}

// bangSFXFor picks the size-matched explosion sound for a rock being
// destroyed.
func bangSFXFor(scale RockScale) SFX {
	switch scale {
	case RockScaleLarge:
		return SFXBangLarge
	case RockScaleMedium:
		return SFXBangMedium
	default:
		return SFXBangSmall
	}
}

// bangSFXForSaucer picks the size-matched explosion sound for a
// saucer being destroyed -- same size-to-sound mapping as
// bangSFXFor's Large/Small ends, there being no Medium saucer.
func bangSFXForSaucer(kind SaucerKind) SFX {
	if kind == SaucerLarge {
		return SFXBangLarge
	}
	return SFXBangSmall
}

// CheckShotSaucerCollision tests every in-flight player shot against
// the saucer (if any is present -- saucer may be nil) for a
// circle-circle overlap. On the first hit, that shot is consumed and
// hit/hitPos report the collision; any remaining shots are returned
// untouched. Doesn't touch the saucer itself, play a sound, or spawn
// an explosion -- the caller owns that plus scoring (see
// Game.updatePlaying), same division of responsibility
// CheckShotRockCollisions has with its own caller.
func CheckShotSaucerCollision(shots []*Shot, saucer *Saucer) (survivingShots []*Shot, hit bool, hitPos Point) {
	if saucer == nil {
		return shots, false, Point{}
	}

	survivingShots = shots[:0]
	for _, s := range shots {
		if !hit && circlesOverlap(s.Pos, shotRadius, saucer.Pos, saucer.Radius()) {
			hit = true
			hitPos = s.Pos
			continue
		}
		survivingShots = append(survivingShots, s)
	}
	return survivingShots, hit, hitPos
}

// CheckShipSaucerCollision reports whether the player's ship overlaps
// the saucer (if any is present). Same visibility/invulnerability
// rules as CheckShipRockCollision, and the same "report only, don't
// touch anything" contract -- the caller (see Game.updateSaucer) owns
// deciding what happens to the ship.
func CheckShipSaucerCollision(ship *PlayerShip, saucer *Saucer) bool {
	if saucer == nil || ship.Hidden() || ship.Invulnerable() {
		return false
	}
	return circlesOverlap(ship.Pos, ship.Radius(), saucer.Pos, saucer.Radius())
}

// CheckRockSaucerCollision reports whether any rock overlaps the
// saucer (if any is present). In the original arcade a saucer that
// collides with an asteroid is destroyed by it -- the rock is left
// alone, mirroring how a ship-rock collision doesn't touch the rock
// either.
func CheckRockSaucerCollision(rocks []*Rock, saucer *Saucer) bool {
	if saucer == nil {
		return false
	}
	for _, r := range rocks {
		if circlesOverlap(r.Pos, r.Radius(), saucer.Pos, saucer.Radius()) {
			return true
		}
	}
	return false
}

// CheckShotShipCollision reports whether any of shots overlaps the
// player's ship -- used for the saucer's own shots, which are the
// only shots in this game that can hit the player (the player has no
// equivalent hazard from its own). Same visibility/invulnerability
// rules as CheckShipRockCollision. Doesn't consume the shot or touch
// ship state -- the caller (see Game.updateSaucer) owns that.
func CheckShotShipCollision(shots []*Shot, ship *PlayerShip) bool {
	if ship.Hidden() || ship.Invulnerable() {
		return false
	}
	for _, s := range shots {
		if circlesOverlap(s.Pos, shotRadius, ship.Pos, ship.Radius()) {
			return true
		}
	}
	return false
}

// circlesOverlap reports whether two circles, given by center and
// radius, intersect.
func circlesOverlap(aPos Point, aRadius float64, bPos Point, bRadius float64) bool {
	return aPos.Sub(bPos).Length() <= aRadius+bRadius
}
