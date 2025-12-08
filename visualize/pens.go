package visualize

import (
	"math"

	"github.com/juruen/rmapi/encoding/rm"
)

// ColorPalette maps Remarkable color IDs to RGB values.
// Based on rmscene's PenColor enum and rmc's RM_PALETTE.
var ColorPalette = map[uint32][3]uint8{
	0:  {0, 0, 0},       // BLACK
	1:  {144, 144, 144}, // GRAY
	2:  {255, 255, 255}, // WHITE
	3:  {251, 247, 25},  // YELLOW
	4:  {0, 255, 0},     // GREEN
	5:  {255, 192, 203}, // PINK
	6:  {78, 105, 201},  // BLUE
	7:  {179, 62, 57},   // RED
	8:  {125, 125, 125}, // GRAY_OVERLAP
	9:  {251, 247, 25},  // HIGHLIGHT (typically yellow)
	10: {161, 216, 125}, // GREEN_2
	11: {139, 208, 229}, // CYAN
	12: {183, 130, 205}, // MAGENTA
	13: {247, 232, 81},  // YELLOW_2
}

// Pen-specific constants
const (
	// Fineliner width multiplier
	finelinerWidthMultiplier = 1.8

	// Mechanical pencil width calculation uses squared brush size
	mechanicalPencilSquaredWidth = true

	// Highlighter fixed width (device units)
	highlighterBaseWidth = 15.0

	// Opacity values
	opacityFull        = 1.0
	opacityPencil      = 0.9
	opacityMechanical  = 0.7
	opacityHighlighter = 0.3

	// Eraser width multiplier
	eraserWidthMultiplier = 2.0

	// Direction to tilt conversion constants
	directionToTiltDivisor = 255.0
	directionToTiltMultiplier = math.Pi * 2
)

// PenRenderer handles rendering properties and calculations for different pen types.
// Each pen type has specific characteristics for width, color, and opacity.
type PenRenderer struct {
	baseWidth   float32
	baseColor   [3]uint8
	baseOpacity float32
	penType     rm.BrushType
}

// NewPenRenderer creates a pen renderer for a given brush type, color, and size.
// It configures pen-specific properties like base width and opacity.
func NewPenRenderer(brushType rm.BrushType, colorID uint32, brushSize rm.BrushSize) *PenRenderer {
	pr := &PenRenderer{
		penType:     brushType,
		baseOpacity: opacityFull,
	}

	// Get color from palette, default to black if not found
	if color, ok := ColorPalette[colorID]; ok {
		pr.baseColor = color
	} else {
		pr.baseColor = ColorPalette[0] // Default to black
	}

	// Set base width based on brush size
	pr.baseWidth = float32(brushSize)

	// Configure pen-specific properties
	switch brushType {
	case rm.Brush, rm.BrushV5:
		pr.baseOpacity = opacityFull

	case rm.BallPoint, rm.BallPointV5:
		pr.baseOpacity = opacityFull

	case rm.Fineliner, rm.FinelinerV5:
		pr.baseWidth = float32(brushSize) * finelinerWidthMultiplier
		pr.baseOpacity = opacityFull

	case rm.Marker, rm.MarkerV5:
		pr.baseOpacity = opacityFull

	case rm.TiltPencil, rm.TiltPencilV5:
		pr.baseOpacity = opacityPencil

	case rm.SharpPencil, rm.SharpPencilV5:
		pr.baseWidth = float32(brushSize) * float32(brushSize)
		pr.baseOpacity = opacityMechanical

	case rm.Highlighter, rm.HighlighterV5:
		pr.baseWidth = highlighterBaseWidth
		pr.baseOpacity = opacityHighlighter

	case rm.Eraser:
		pr.baseWidth = float32(brushSize) * eraserWidthMultiplier
		pr.baseColor = ColorPalette[2] // White
		pr.baseOpacity = opacityFull

	case rm.EraseArea:
		pr.baseColor = ColorPalette[2] // White
		pr.baseOpacity = 0.0
	}

	return pr
}

// GetStrokeWidth calculates the stroke width for a point based on pen type.
// Different pens respond differently to pressure, speed, direction, and point width.
func (pr *PenRenderer) GetStrokeWidth(speed, direction, width, pressure float32) float32 {
	switch pr.penType {
	case rm.Brush, rm.BrushV5:
		return pr.calculateBrushWidth(speed, direction, width, pressure)

	case rm.BallPoint, rm.BallPointV5:
		return pr.calculateBallpointWidth(speed, width, pressure)

	case rm.Marker, rm.MarkerV5:
		return pr.calculateMarkerWidth(direction, width)

	case rm.TiltPencil, rm.TiltPencilV5:
		return pr.calculatePencilWidth(speed, direction, width, pressure)

	case rm.SharpPencil, rm.SharpPencilV5:
		return pr.baseWidth

	default:
		// Default: use base width with pressure variation
		return pr.baseWidth * (0.5 + pressure*0.5)
	}
}

// GetStrokeColor calculates the stroke color for a point.
// Some pens (like brush and ballpoint) vary color intensity based on pressure and speed.
func (pr *PenRenderer) GetStrokeColor(speed, direction, width, pressure float32) [3]uint8 {
	switch pr.penType {
	case rm.Brush, rm.BrushV5:
		return pr.calculateBrushColor(speed, pressure)

	case rm.BallPoint, rm.BallPointV5:
		return pr.calculateBallpointColor(speed, pressure)

	default:
		return pr.baseColor
	}
}

// GetStrokeOpacity calculates the stroke opacity for a point.
// Pencils vary opacity based on pressure, while highlighters use fixed low opacity.
func (pr *PenRenderer) GetStrokeOpacity(speed, direction, width, pressure float32) float32 {
	switch pr.penType {
	case rm.TiltPencil, rm.TiltPencilV5:
		return pr.calculatePencilOpacity(speed, pressure)

	case rm.Highlighter, rm.HighlighterV5:
		return 0.2 // Fixed low opacity for highlighters

	case rm.SharpPencil, rm.SharpPencilV5:
		return pr.baseOpacity

	case rm.EraseArea:
		return 0.0

	default:
		return pr.baseOpacity
	}
}

// calculateBrushWidth calculates width for brush pen type.
// Brush width varies significantly with pressure, tilt (direction), and speed.
func (pr *PenRenderer) calculateBrushWidth(speed, direction, width, pressure float32) float32 {
	tilt := float32(directionToTilt(direction))
	return 0.7 * (((1 + (1.4 * pressure)) * (width / 4)) -
		(0.5 * tilt) - ((speed / 4) / 50))
}

// calculateBallpointWidth calculates width for ballpoint pen type.
// Ballpoint width varies with pressure and speed.
func (pr *PenRenderer) calculateBallpointWidth(speed, width, pressure float32) float32 {
	return (0.5 + pressure) + (width / 4) - 0.5*((speed/4)/50)
}

// calculateMarkerWidth calculates width for marker pen type.
// Marker width varies with tilt (direction).
func (pr *PenRenderer) calculateMarkerWidth(direction, width float32) float32 {
	tilt := float32(directionToTilt(direction))
	return 0.9 * ((width / 4) - 0.4*tilt)
}

// calculatePencilWidth calculates width for pencil pen type.
// Pencil width varies significantly with pressure, tilt, and speed, with a maximum limit.
func (pr *PenRenderer) calculatePencilWidth(speed, direction, width, pressure float32) float32 {
	tilt := directionToTilt(direction)
	segmentWidth := 0.7 * ((((0.8 * pr.baseWidth) + (0.5 * pressure)) * (width / 4)) -
		(0.25 * float32(math.Pow(tilt, 1.8))) - (0.6 * ((speed / 4) / 50)))
	
	maxWidth := pr.baseWidth * 10
	if segmentWidth > maxWidth {
		segmentWidth = maxWidth
	}
	return segmentWidth
}

// calculateBrushColor calculates color intensity for brush pen type.
// Brush color intensity varies with pressure and speed, creating a watercolor effect.
func (pr *PenRenderer) calculateBrushColor(speed, pressure float32) [3]uint8 {
	intensity := float64((math.Pow(float64(pressure), 1.5) - 0.2*((float64(speed)/4)/50)) * 1.5)
	intensity = clamp(intensity)
	revIntensity := math.Abs(intensity - 1)
	
	return [3]uint8{
		uint8(revIntensity * float64(255-pr.baseColor[0])),
		uint8(revIntensity * float64(255-pr.baseColor[1])),
		uint8(revIntensity * float64(255-pr.baseColor[2])),
	}
}

// calculateBallpointColor calculates grayscale color for ballpoint pen type.
// Ballpoint creates grayscale intensity based on pressure and speed.
func (pr *PenRenderer) calculateBallpointColor(speed, pressure float32) [3]uint8 {
	intensity := float64((0.1 * -((speed / 4) / 35)) + (1.2 * pressure) + 0.5)
	intensity = clamp(intensity)
	gray := uint8(math.Min(math.Abs(intensity-1)*255, 60))
	return [3]uint8{gray, gray, gray}
}

// calculatePencilOpacity calculates opacity for pencil pen type.
// Pencil opacity varies with pressure and speed, creating a natural pencil effect.
func (pr *PenRenderer) calculatePencilOpacity(speed, pressure float32) float32 {
	opacity := float64((0.1 * -((speed / 4) / 35)) + (1.0 * pressure))
	opacity = clamp(opacity) - 0.1
	if opacity < 0 {
		opacity = 0
	}
	return float32(opacity)
}

// directionToTilt converts direction value (0-255) to tilt angle in radians.
// This is used for pens that respond to pen tilt (like brush and marker).
func directionToTilt(direction float32) float64 {
	return float64(direction) * directionToTiltMultiplier / directionToTiltDivisor
}

// clamp clamps a value between 0 and 1.
func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

