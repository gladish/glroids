package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// font5x7 defines the glyphs the HUD/menu screens need as 7 rows of a
// 5-bit-wide pattern (bit 4 = leftmost column, bit 0 = rightmost).
// drawText stencils these as filled squares via vector.FillRect --
// that keeps every bit of text rendering inside this package's
// existing vector-line-art toolbox instead of pulling in a
// font-rendering dependency (ebiten's text/v2 package, and any font
// source for it) for a handful of short strings.
//
// Only the runes actually used by main.go's HUD/menu text are
// defined -- an unsupported rune is silently skipped by drawText
// rather than falling back to a placeholder glyph.
var font5x7 = map[rune][7]uint8{
	' ': {0b00000, 0b00000, 0b00000, 0b00000, 0b00000, 0b00000, 0b00000},
	'A': {0b01110, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'C': {0b01111, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b01111},
	'D': {0b11110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b11110},
	'E': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b11111},
	'F': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b10000},
	'G': {0b01111, 0b10000, 0b10000, 0b10111, 0b10001, 0b10001, 0b01111},
	'H': {0b10001, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'I': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b11111},
	'L': {0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b11111},
	'M': {0b10001, 0b11011, 0b10101, 0b10001, 0b10001, 0b10001, 0b10001},
	'N': {0b10001, 0b11001, 0b10101, 0b10011, 0b10001, 0b10001, 0b10001},
	'O': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'P': {0b11110, 0b10001, 0b10001, 0b11110, 0b10000, 0b10000, 0b10000},
	'R': {0b11110, 0b10001, 0b10001, 0b11110, 0b10100, 0b10010, 0b10001},
	'S': {0b01111, 0b10000, 0b10000, 0b01110, 0b00001, 0b00001, 0b11110},
	'T': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100},
	'U': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'V': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01010, 0b00100},
	'W': {0b10001, 0b10001, 0b10001, 0b10101, 0b10101, 0b11011, 0b10001},
	'Y': {0b10001, 0b10001, 0b01010, 0b00100, 0b00100, 0b00100, 0b00100},
	'0': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'1': {0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'2': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b01000, 0b11111},
	'3': {0b01110, 0b10001, 0b00001, 0b01110, 0b00001, 0b10001, 0b01110},
	'4': {0b00010, 0b00110, 0b01010, 0b10010, 0b11111, 0b00010, 0b00010},
	'5': {0b11111, 0b10000, 0b11110, 0b00001, 0b00001, 0b10001, 0b01110},
	'6': {0b01110, 0b10000, 0b10000, 0b11110, 0b10001, 0b10001, 0b01110},
	'7': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	'8': {0b01110, 0b10001, 0b10001, 0b01110, 0b10001, 0b10001, 0b01110},
	'9': {0b01110, 0b10001, 0b10001, 0b01111, 0b00001, 0b00001, 0b01110},
}

// glyphCols/glyphRows are the fixed dimensions every font5x7 entry is
// authored at.
const glyphCols = 5
const glyphRows = 7

// glyphSpacing is the gap, in glyph "pixels", left between adjacent
// glyphs when drawText lays out a string.
const glyphSpacing = 1

// hudTextScale/titleTextScale/promptTextScale are how large each
// glyph pixel is drawn, in screen pixels, for the HUD's small
// lives/wave readout versus the attract/game-over screens' bigger
// title and prompt lines.
const hudTextScale = 3.0
const titleTextScale = 6.0
const promptTextScale = 3.0

// textWidth returns how wide s renders at the given pixel scale,
// including inter-glyph spacing -- used by drawCenteredText to find
// where a line's left edge should land.
func textWidth(s string, pixel float64) float64 {
	if len(s) == 0 {
		return 0
	}
	glyphWidth := float64(glyphCols) * pixel
	spacing := float64(glyphSpacing) * pixel
	return float64(len(s))*glyphWidth + float64(len(s)-1)*spacing
}

// drawText stamps s onto screen as blocky font5x7 glyphs, each glyph
// pixel scale screen-units square. x is the left edge of the first
// glyph; y is the vertical center of the whole line. Runes not in
// font5x7 (there shouldn't be any, for the strings this game draws)
// are skipped rather than drawn as a placeholder box.
func drawText(screen *ebiten.Image, s string, x, y, scale float64, clr color.Color) {
	glyphWidth := float64(glyphCols) * scale
	advance := glyphWidth + float64(glyphSpacing)*scale
	top := y - (float64(glyphRows)*scale)/2

	cursor := x
	for _, r := range s {
		rows, ok := font5x7[r]
		if ok {
			for row := 0; row < glyphRows; row++ {
				bits := rows[row]
				for col := 0; col < glyphCols; col++ {
					if bits&(1<<uint(glyphCols-1-col)) == 0 {
						continue
					}
					px := cursor + float64(col)*scale
					py := top + float64(row)*scale
					vector.FillRect(screen, float32(px), float32(py), float32(scale), float32(scale), clr, false)
				}
			}
		}
		cursor += advance
	}
}

// drawCenteredText draws s horizontally centered on the screen, its
// line vertically centered on y.
func drawCenteredText(screen *ebiten.Image, s string, y, scale float64, clr color.Color) {
	x := screenWidth/2 - textWidth(s, scale)/2
	drawText(screen, s, x, y, scale, clr)
}

// livesIconScale/Spacing/Margin size and position the small
// ship-outline icons the HUD draws for each life still in reserve.
// Reuses PlayerShip's own outline via TransformPath/strokeClosedPath
// rather than a separate icon asset, so it always matches whatever
// the ship itself looks like.
const livesIconScale = 0.6
const livesIconSpacing = 22.0
const livesIconMargin = 24.0

// drawLives stamps one small upright ship icon per extra life --
// lives-1, since the ship currently in play isn't counted -- along
// the top-left of the screen.
func drawLives(screen *ebiten.Image, ship *PlayerShip, lives int) {
	extra := lives - 1
	if extra <= 0 {
		return
	}

	scaled := make([]Point, len(ship.Path))
	for i, p := range ship.Path {
		scaled[i] = p.Scale(livesIconScale)
	}

	for i := 0; i < extra; i++ {
		pos := Point{X: livesIconMargin + float64(i)*livesIconSpacing, Y: livesIconMargin}
		strokeClosedPath(screen, TransformPath(scaled, 0, pos))
	}
}
