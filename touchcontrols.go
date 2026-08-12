package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// TouchInput mirrors KeyBindings' five actions, sourced from the
// on-screen touch buttons instead of the keyboard -- see
// currentTouchInput. The zero value is "no touch input at all," so
// PlayerShip.Update can just OR each field in alongside its existing
// keyboard checks without needing a separate enabled flag threaded
// through -- on desktop, every field simply stays false forever.
//
// TurnLeft/TurnRight/Thrust are held-style, checked fresh every tick
// (mirrors ebiten.IsKeyPressed). Fire/Hyperspace are edge-triggered,
// true only the tick a button was newly pressed (mirrors
// inpututil.IsKeyJustPressed) -- holding a finger on Fire shouldn't
// spray shots any more than holding Space does.
type TouchInput struct {
	TurnLeft   bool
	TurnRight  bool
	Thrust     bool
	Fire       bool
	Hyperspace bool
}

// touchButton is one on-screen control: a circle at Center/Radius
// (world/screen coordinates -- Layout always reports the current
// screenWidth x screenHeight 1:1, so these never need device-pixel
// conversion) drawn with a single-letter Label. Center is computed by
// layoutTouchButtons from whatever screenWidth/screenHeight are in
// effect, so it stays correct across the landscape/touch aspect
// switch.
type touchButton struct {
	Center Point
	Radius float64
	Label  string
}

// Hit reports whether p (a touch position) lands inside the button.
func (b touchButton) Hit(p Point) bool {
	return p.Sub(b.Center).Length() <= b.Radius
}

// Button layout: turn/thrust cluster in the bottom-left corner, fire
// (bigger -- it's used constantly) and hyperspace (smaller, tucked in
// the corner -- same "emergency escape hatch" role it plays in the
// original arcade) in the bottom-right.
const (
	touchButtonRadius   = 44.0
	touchFireRadius     = 54.0
	touchHyperRadius    = 34.0
	touchMarginX        = 90.0
	touchMarginBottom   = 90.0
	touchClusterSpacing = 110.0
)

var (
	touchTurnLeft   touchButton
	touchTurnRight  touchButton
	touchThrust     touchButton
	touchFire       touchButton
	touchHyperspace touchButton
)

// layoutTouchButtons (re)computes every on-screen button's position
// from the current screenWidth/screenHeight. Run once at package init
// (for the landscape default) and again by Game.enableTouchControls
// when the canvas switches to its portrait touch aspect, so the
// cluster stays anchored to the bottom corners of whichever canvas
// size is actually in play rather than the landscape layout baked in
// at startup.
func layoutTouchButtons() {
	touchTurnLeft = touchButton{
		Center: Point{X: touchMarginX, Y: screenHeight - touchMarginBottom},
		Radius: touchButtonRadius,
		Label:  "L",
	}
	touchTurnRight = touchButton{
		Center: Point{X: touchMarginX + touchClusterSpacing, Y: screenHeight - touchMarginBottom},
		Radius: touchButtonRadius,
		Label:  "R",
	}
	touchThrust = touchButton{
		Center: Point{X: touchMarginX + touchClusterSpacing/2, Y: screenHeight - touchMarginBottom - touchClusterSpacing},
		Radius: touchButtonRadius,
		Label:  "T",
	}
	touchFire = touchButton{
		Center: Point{X: screenWidth - touchMarginX, Y: screenHeight - touchMarginBottom},
		Radius: touchFireRadius,
		Label:  "F",
	}
	touchHyperspace = touchButton{
		Center: Point{X: screenWidth - touchMarginX - touchClusterSpacing, Y: screenHeight - touchMarginBottom - 20},
		Radius: touchHyperRadius,
		Label:  "H",
	}
}

func init() {
	layoutTouchButtons()
}

// touchJustPressed reports whether any new touch began this tick --
// used as a tap-anywhere fallback everywhere Enter currently confirms
// a screen (see updateAttract/updateGameOver), and as updateAttract's
// signal to switch the on-screen buttons on for the rest of the
// session.
func touchJustPressed() bool {
	return len(inpututil.AppendJustPressedTouchIDs(nil)) > 0
}

// currentTouchInput hit-tests every currently active touch (for the
// held-style turn/thrust buttons) and every touch that just began
// this tick (for the edge-triggered fire/hyperspace buttons) against
// the button layout above. Safe to call every tick regardless of
// whether the on-screen controls are even being drawn --
// ebiten.AppendTouchIDs simply returns nothing on a device with no
// touchscreen, so this costs nothing on desktop.
func currentTouchInput() TouchInput {
	var in TouchInput

	for _, id := range ebiten.AppendTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		p := Point{X: float64(x), Y: float64(y)}
		if touchTurnLeft.Hit(p) {
			in.TurnLeft = true
		}
		if touchTurnRight.Hit(p) {
			in.TurnRight = true
		}
		if touchThrust.Hit(p) {
			in.Thrust = true
		}
	}

	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		p := Point{X: float64(x), Y: float64(y)}
		if touchFire.Hit(p) {
			in.Fire = true
		}
		if touchHyperspace.Hit(p) {
			in.Hyperspace = true
		}
	}

	return in
}

// touchControlColor is a dim, semi-transparent white so the overlay
// reads clearly as UI without competing with the (fully opaque white)
// ship/rock/shot art underneath it.
var touchControlColor = color.RGBA{R: 255, G: 255, B: 255, A: 100}

// touchLabelScale sizes each button's letter -- bigger than the HUD
// text (see hudTextScale) so it stays legible at arm's length on a
// phone screen.
const touchLabelScale = 5.0

// drawTouchControls draws every on-screen button as a stroked circle
// (matching the rest of the game's unfilled vector-line-art look)
// with its letter centered inside.
func drawTouchControls(screen *ebiten.Image) {
	for _, b := range []touchButton{touchTurnLeft, touchTurnRight, touchThrust, touchFire, touchHyperspace} {
		vector.StrokeCircle(screen, float32(b.Center.X), float32(b.Center.Y), float32(b.Radius), 2, touchControlColor, true)
		labelX := b.Center.X - textWidth(b.Label, touchLabelScale)/2
		drawText(screen, b.Label, labelX, b.Center.Y, touchLabelScale, touchControlColor)
	}
}
