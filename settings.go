package main

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// KeyBindings holds every keyboard key the player can remap for
// gameplay. It's the single place movement/fire code should read
// input from -- reading ebiten.Key* constants directly at the call
// site would scatter "what key does what" across the codebase and
// make it unclear which keys are actually meant to be reconfigurable
// (the debug/test-sound keys in main.go aren't, so they're left
// reading ebiten.Key* directly).
type KeyBindings struct {
	TurnLeft   ebiten.Key
	TurnRight  ebiten.Key
	Thrust     ebiten.Key
	Fire       ebiten.Key
	Hyperspace ebiten.Key
}

// DefaultKeyBindings is the out-of-the-box control scheme.
func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
		TurnLeft:   ebiten.KeyLeft,
		TurnRight:  ebiten.KeyRight,
		Thrust:     ebiten.KeyUp,
		Fire:       ebiten.KeySpace,
		Hyperspace: ebiten.KeyC,
	}
}

// defaultRespawnHiddenDuration is how long the ship stays hidden
// after a fatal hyperspace jump before it reappears.
const defaultRespawnHiddenDuration = 500 * time.Millisecond

// Settings collects every user-configurable option. Keyboard bindings
// and respawn-hidden time are the categories so far -- volume,
// difficulty, etc. can join this struct later without changing the
// call sites that already read Settings.
type Settings struct {
	Keys KeyBindings

	// RespawnHiddenDuration is how long the ship stays hidden after a
	// fatal hyperspace jump (see PlayerShip.respawn) before it
	// reappears.
	RespawnHiddenDuration time.Duration
}

// DefaultSettings is what a fresh game starts with.
func DefaultSettings() Settings {
	return Settings{
		Keys:                  DefaultKeyBindings(),
		RespawnHiddenDuration: defaultRespawnHiddenDuration,
	}
}
