package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"io"
	"log"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

const sampleRate = 44100

// ---- Embedded WAV assets ----
// Embedding keeps every sound inside the binary; combined with fully
// decoding to PCM at startup (below), there's zero file I/O and zero
// wav-decode cost left at play time.

//go:embed sounds/bangLarge.wav
var wavBangLarge []byte

//go:embed sounds/bangMedium.wav
var wavBangMedium []byte

//go:embed sounds/bangSmall.wav
var wavBangSmall []byte

//go:embed sounds/beat1.wav
var wavBeat1 []byte

//go:embed sounds/beat2.wav
var wavBeat2 []byte

//go:embed sounds/extraShip.wav
var wavExtraShip []byte

//go:embed sounds/fire.wav
var wavFire []byte

//go:embed sounds/saucerBig.wav
var wavSaucerBig []byte

//go:embed sounds/saucerSmall.wav
var wavSaucerSmall []byte

//go:embed sounds/thrust.wav
var wavThrust []byte

// SFX identifies a sound effect.
type SFX int

const (
	SFXFire SFX = iota
	SFXBangLarge
	SFXBangMedium
	SFXBangSmall
	SFXBeat1
	SFXBeat2
	SFXExtraShip
	SFXThrustLoop
	SFXSaucerBigLoop
	SFXSaucerSmallLoop
)

// oneShot is a pool of pre-built players for a sound that can be
// re-triggered, possibly overlapping itself (fire, explosions, ...).
type oneShot struct {
	players []*audio.Player
	next    int // round-robin cursor, used only when every voice is busy
}

// SoundManager owns a fixed set of pre-decoded sounds and pre-built
// players. Nothing is decoded or allocated once NewSoundManager
// returns -- Play/PlayLoop just flip existing players on and off,
// which is what keeps repeated/overlapping playback glitch-free and
// low-latency. The trade is memory: every voice holds its own player
// object up front instead of being built on demand.
type SoundManager struct {
	ctx      *audio.Context
	oneShots map[SFX]*oneShot
	loops    map[SFX]*loopVoice
}

// loopFadeSteps is how many SoundManager.Update() calls a loop's
// stop fade-out is spread across. Cutting a loop with Pause() lands
// on some arbitrary sample value rather than a zero-crossing, which
// is heard as a click/pop -- worse on some stops than others,
// depending on where in the waveform playback happened to be. Ramping
// the volume down first removes the discontinuity instead of just
// relocating it.
const loopFadeSteps = 6

// loopVoice wraps a looping player with its target volume and any
// in-progress fade-out state.
type loopVoice struct {
	player *audio.Player
	volume float64 // volume to restore to once idle/playing again
	fading int     // >0 while fading out; counts down to 0
}

// poolSizes controls how many overlapping instances of each one-shot
// sound are pre-built. Sounds that can be triggered in a rapid burst
// (fire, explosions) get a bigger pool; rare one-shots get by with a
// couple of voices. All voices for a sound share the same underlying
// PCM byte slice, so a bigger pool is cheap.
var poolSizes = map[SFX]int{
	SFXFire:       8,
	SFXBangLarge:  4,
	SFXBangMedium: 4,
	SFXBangSmall:  4,
	SFXBeat1:      2,
	SFXBeat2:      2,
	SFXExtraShip:  2,
}

var oneShotSources = map[SFX][]byte{
	SFXFire:       wavFire,
	SFXBangLarge:  wavBangLarge,
	SFXBangMedium: wavBangMedium,
	SFXBangSmall:  wavBangSmall,
	SFXBeat1:      wavBeat1,
	SFXBeat2:      wavBeat2,
	SFXExtraShip:  wavExtraShip,
}

var loopSources = map[SFX][]byte{
	SFXThrustLoop:      wavThrust,
	SFXSaucerBigLoop:   wavSaucerBig,
	SFXSaucerSmallLoop: wavSaucerSmall,
}

// NewSoundManager decodes every embedded wav to raw PCM once and
// builds all the players up front. This is the "pay with memory"
// trade: every voice is a ready-to-go *audio.Player before the game
// loop ever calls Play, so triggering a sound is just Rewind+Play on
// an existing object -- no decode, no allocation, minimal latency.
func NewSoundManager(ctx *audio.Context) *SoundManager {
	sm := &SoundManager{
		ctx:      ctx,
		oneShots: make(map[SFX]*oneShot, len(oneShotSources)),
		loops:    make(map[SFX]*loopVoice, len(loopSources)),
	}

	for id, raw := range oneShotSources {
		pcm := decodeToPCM(ctx, raw)
		n := poolSizes[id]
		if n < 1 {
			n = 1
		}
		os := &oneShot{players: make([]*audio.Player, n)}
		for i := range os.players {
			// NewPlayerFromBytes lets many players safely share one
			// []byte -- that's the whole trick behind the pool.
			os.players[i] = ctx.NewPlayerFromBytes(pcm)
		}
		sm.oneShots[id] = os
	}

	for id, raw := range loopSources {
		pcm := decodeToPCM(ctx, raw)
		// Source wavs often carry a sliver of silence at the head
		// and/or tail. Looping the buffer's full length verbatim
		// loops that silence too, which is heard as a gap right at
		// the wrap point. Trim it (and feather the cut) so the wrap
		// is seamless.
		pcm = trimLoopSilence(pcm)
		loop := audio.NewInfiniteLoop(bytes.NewReader(pcm), int64(len(pcm)))
		p, err := ctx.NewPlayer(loop)
		if err != nil {
			log.Fatalf("sound: build loop player: %v", err)
		}
		sm.loops[id] = &loopVoice{player: p, volume: 1}
	}

	return sm
}

// PCM here is signed 16-bit little endian, 2 channels (as required by
// audio.NewPlayer / audio.NewInfiniteLoop).
const pcmFrameSize = 4 // 2 channels * 2 bytes

// trimLoopSilence strips near-silent lead-in/lead-out frames from a
// stereo 16-bit PCM buffer intended for looping, and feathers a short
// fade across the new edges so the cut doesn't introduce a click.
// Without this, a wav with even a few ms of silence baked in at the
// start or end gets looped verbatim -- audible as a "dead air" gap
// between one pass of the loop and the next.
func trimLoopSilence(pcm []byte) []byte {
	const (
		silenceThresh = 500 // amplitude out of a possible 32767
		fadeFrames    = 128 // ~3ms at 44.1kHz, just enough to avoid a click
	)

	frames := len(pcm) / pcmFrameSize
	if frames == 0 {
		return pcm
	}

	start := 0
	for start < frames && frameAmplitude(pcm, start) < silenceThresh {
		start++
	}
	end := frames
	for end > start && frameAmplitude(pcm, end-1) < silenceThresh {
		end--
	}

	// The clip is silent throughout (or too short to bother) --
	// leave it as-is rather than trim it to nothing.
	if end-start < fadeFrames*2 {
		return pcm
	}

	trimmed := make([]byte, (end-start)*pcmFrameSize)
	copy(trimmed, pcm[start*pcmFrameSize:end*pcmFrameSize])
	fadeEdges(trimmed, fadeFrames)

	return trimmed
}

// frameAmplitude returns the louder of the two channels' absolute
// sample values for the given frame index.
func frameAmplitude(pcm []byte, frame int) int {
	i := frame * pcmFrameSize
	l := int(int16(binary.LittleEndian.Uint16(pcm[i : i+2])))
	r := int(int16(binary.LittleEndian.Uint16(pcm[i+2 : i+4])))
	if l < 0 {
		l = -l
	}
	if r < 0 {
		r = -r
	}
	if l > r {
		return l
	}
	return r
}

// fadeEdges applies a linear fade-in over the first fadeFrames frames
// and a linear fade-out over the last fadeFrames frames of pcm.
func fadeEdges(pcm []byte, fadeFrames int) {
	total := len(pcm) / pcmFrameSize
	if fadeFrames > total/2 {
		fadeFrames = total / 2
	}
	for f := 0; f < fadeFrames; f++ {
		gain := float64(f) / float64(fadeFrames)
		scaleFrame(pcm, f, gain)
		scaleFrame(pcm, total-1-f, gain)
	}
}

func scaleFrame(pcm []byte, frame int, gain float64) {
	i := frame * pcmFrameSize
	for ch := 0; ch < 2; ch++ {
		off := i + ch*2
		s := int16(binary.LittleEndian.Uint16(pcm[off : off+2]))
		s = int16(float64(s) * gain)
		binary.LittleEndian.PutUint16(pcm[off:off+2], uint16(s))
	}
}

func decodeToPCM(ctx *audio.Context, raw []byte) []byte {
	stream, err := wav.DecodeWithSampleRate(ctx.SampleRate(), bytes.NewReader(raw))
	if err != nil {
		log.Fatalf("sound: decode wav: %v", err)
	}
	pcm, err := io.ReadAll(stream)
	if err != nil {
		log.Fatalf("sound: read pcm: %v", err)
	}
	return pcm
}

// Play triggers a one-shot sound effect. It picks the first idle
// voice in the pool; if every voice happens to be busy (a rapid-fire
// burst bigger than the pool) it steals the oldest one round-robin
// rather than dropping the sound or allocating a new player.
func (sm *SoundManager) Play(id SFX) {
	os, ok := sm.oneShots[id]
	if !ok {
		return
	}
	for _, p := range os.players {
		if !p.IsPlaying() {
			_ = p.Rewind()
			p.Play()
			return
		}
	}
	p := os.players[os.next]
	os.next = (os.next + 1) % len(os.players)
	_ = p.Rewind()
	p.Play()
}

// PlayLoop starts a looping sound (e.g. thrust) if it isn't already
// playing. Safe to call every frame while a key/condition is held.
// If the loop is mid fade-out (StopLoop was just called), this cancels
// the fade and snaps back to full volume without restarting playback,
// so rapid tap-release-tap doesn't cause a stutter or double-trigger.
func (sm *SoundManager) PlayLoop(id SFX) {
	lv, ok := sm.loops[id]
	if !ok {
		return
	}
	if lv.fading > 0 {
		lv.fading = 0
		lv.player.SetVolume(lv.volume)
	}
	if lv.player.IsPlaying() {
		return
	}
	lv.player.Play()
}

// StopLoop begins fading a looping sound out; SoundManager.Update
// finishes the job by pausing and rewinding it once the fade
// completes. Cutting a loop instantly with Pause() truncates the
// waveform at an arbitrary (usually non-zero) sample, which is heard
// as a click -- fading first avoids that.
func (sm *SoundManager) StopLoop(id SFX) {
	lv, ok := sm.loops[id]
	if !ok || !lv.player.IsPlaying() || lv.fading > 0 {
		return
	}
	lv.fading = loopFadeSteps
}

// Update advances any in-progress loop fade-outs. Call this once per
// game tick (e.g. from Game.Update) alongside Play/PlayLoop/StopLoop.
func (sm *SoundManager) Update() {
	for _, lv := range sm.loops {
		if lv.fading <= 0 {
			continue
		}
		lv.fading--
		if lv.fading == 0 {
			lv.player.Pause()
			_ = lv.player.Rewind()
			lv.player.SetVolume(lv.volume)
			continue
		}
		lv.player.SetVolume(lv.volume * float64(lv.fading) / float64(loopFadeSteps))
	}
}

func (sm *SoundManager) IsLoopPlaying(id SFX) bool {
	lv, ok := sm.loops[id]
	return ok && lv.player.IsPlaying()
}

// SetVolume scales one sound (0..1). For a one-shot this sets every
// pooled voice; for a loop it sets the single player (unless a
// fade-out is in progress, in which case it takes effect once the
// fade finishes or is cancelled by PlayLoop).
func (sm *SoundManager) SetVolume(id SFX, v float64) {
	if lv, ok := sm.loops[id]; ok {
		lv.volume = v
		if lv.fading == 0 {
			lv.player.SetVolume(v)
		}
		return
	}
	if os, ok := sm.oneShots[id]; ok {
		for _, p := range os.players {
			p.SetVolume(v)
		}
	}
}
