package main

// CheckShotRockCollisions tests every in-flight player shot against
// every rock for a circle-circle overlap. On a hit: the shot is
// consumed, the rock is destroyed and replaced by SplitRock's
// children (nothing, if it was already Small), the size-matched bang
// sound plays -- same sounds the debug keys in main.go trigger
// manually -- and a spark burst spawns at the shot's position (the
// point of impact). A shot stops at its first hit rather than
// piercing through to a second rock the same tick; separate shots can
// each still score their own hit in one tick.
//
// Returns the surviving shots, the rock field with destroyed rocks
// removed and any split children appended, and any new Explosions
// spawned this call -- the caller owns adding those to whatever slice
// Game.Update/Draw drives.
func CheckShotRockCollisions(shots []*Shot, rocks []*Rock, sm *SoundManager) ([]*Shot, []*Rock, []*Explosion) {
	survivingShots := shots[:0]
	var newExplosions []*Explosion

shotLoop:
	for _, s := range shots {
		for i, r := range rocks {
			if !circlesOverlap(s.Pos, shotRadius, r.Pos, r.Radius()) {
				continue
			}

			sm.Play(bangSFXFor(r.Scale))
			newExplosions = append(newExplosions, NewExplosion(s.Pos))

			// Swap-remove the hit rock, then append whatever it split
			// into (nothing, for a Small rock).
			rocks[i] = rocks[len(rocks)-1]
			rocks = rocks[:len(rocks)-1]
			rocks = append(rocks, SplitRock(r)...)

			continue shotLoop // this shot is spent; don't test it against the rest
		}
		survivingShots = append(survivingShots, s)
	}

	return survivingShots, rocks, newExplosions
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

// circlesOverlap reports whether two circles, given by center and
// radius, intersect.
func circlesOverlap(aPos Point, aRadius float64, bPos Point, bRadius float64) bool {
	return aPos.Sub(bPos).Length() <= aRadius+bRadius
}
