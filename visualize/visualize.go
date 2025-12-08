package visualize

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/encoding/rm"
)

// Visualization constants
const (
	defaultOutputWidth     = 1404  // ReMarkable2 width in pixels
	defaultPaddingPercent = 0.05  // 5% padding around content
	defaultMinPadding      = 50    // Minimum padding in pixels
	defaultStrokeWidthScale = 0.25 // Scaling factor for stroke width
	defaultMinStrokeWidth  = 1     // Minimum stroke width in pixels
	defaultMaxStrokeWidth  = 8     // Maximum stroke width in pixels
	minImageHeight         = 100   // Minimum image height in pixels

	// Highlighter-specific constants
	highlighterBaseWidthPixels = 15.0  // Base width for highlighters
	highlighterWidthMultiplier = 4.0  // Multiplier for highlighter thickness
	highlighterMinWidth        = 20   // Minimum highlighter width
	highlighterMaxWidth        = 100  // Maximum highlighter width
	highlighterOpacity         = float32(0.45) // Highlighter opacity (intermediate transparency)
	highlighterColorLighten    = 0.7  // Color lightening factor (mix with white)
	highlighterWhiteMix        = 0.3  // White mixing factor for pastel effect
)

// VisualizationConfig holds configuration for rendering strokes to PNG.
type VisualizationConfig struct {
	// OutputWidth is the fixed width of the output image in pixels (default: 1404 for ReMarkable2)
	OutputWidth int
	// PaddingPercent is the percentage of padding to add around content (default: 0.05 = 5%)
	PaddingPercent float32
	// MinPadding is the minimum padding in pixels (default: 50)
	MinPadding float32
	// StrokeWidthScale is the scaling factor for stroke width (default: 0.25)
	StrokeWidthScale float32
	// MinStrokeWidth is the minimum stroke width in pixels (default: 1)
	MinStrokeWidth int
	// MaxStrokeWidth is the maximum stroke width in pixels (default: 8)
	MaxStrokeWidth int
}

// DefaultVisualizationConfig returns a config with ReMarkable2 defaults.
func DefaultVisualizationConfig() VisualizationConfig {
	return VisualizationConfig{
		OutputWidth:      defaultOutputWidth,
		PaddingPercent:   defaultPaddingPercent,
		MinPadding:       defaultMinPadding,
		StrokeWidthScale: defaultStrokeWidthScale,
		MinStrokeWidth:   defaultMinStrokeWidth,
		MaxStrokeWidth:   defaultMaxStrokeWidth,
	}
}

// VisualizePage renders a page's strokes to a PNG file using default configuration.
func VisualizePage(zip *archive.Zip, pageNumber int, outputPath string) error {
	return VisualizePageWithConfig(zip, pageNumber, outputPath, DefaultVisualizationConfig())
}

// VisualizePageWithConfig renders a page's strokes to a PNG file with custom configuration.
// The output image has a fixed width (typically 1404px for ReMarkable2) and dynamic height
// based on the content, maintaining aspect ratio.
func VisualizePageWithConfig(zip *archive.Zip, pageNumber int, outputPath string, config VisualizationConfig) error {
	if pageNumber < 0 || pageNumber >= len(zip.Pages) {
		return nil
	}

	page := zip.Pages[pageNumber]
	if page.Data == nil {
		return nil
	}

	// Calculate bounding box of all strokes
	bbox := calculateBoundingBox(page.Data, config)
	if bbox == nil {
		return createEmptyImage(outputPath, config.OutputWidth, minImageHeight)
	}

	// Calculate scale factors and image dimensions
	scaleX, scaleY, imgWidth, imgHeight := calculateImageDimensions(bbox, config)
	if imgHeight < minImageHeight {
		imgHeight = minImageHeight
	}

	// Create image and fill with white background
	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	fillWhiteBackground(img, imgWidth, imgHeight)

	// Draw all strokes (highlighters first, then other strokes)
	drawStrokes(img, page.Data, bbox, scaleX, scaleY, imgWidth, imgHeight, config)

	// Save PNG
	return savePNG(img, outputPath)
}

// boundingBox represents the bounding box of strokes with padding.
type boundingBox struct {
	minX, minY, maxX, maxY float32
	paddingX, paddingY     float32
}

// calculateBoundingBox calculates the bounding box of all strokes in the page.
// Returns nil if no valid strokes are found.
func calculateBoundingBox(pageData *rm.Rm, config VisualizationConfig) *boundingBox {
	var minX, minY, maxX, maxY float32
	hasPoints := false

	// Find min/max coordinates across all strokes
	for _, layer := range pageData.Layers {
		for _, line := range layer.Lines {
			if line.BrushType == rm.EraseArea || len(line.Points) < 2 {
				continue
			}

			for _, point := range line.Points {
				if !hasPoints {
					minX, minY = point.X, point.Y
					maxX, maxY = point.X, point.Y
					hasPoints = true
				} else {
					if point.X < minX {
						minX = point.X
					}
					if point.X > maxX {
						maxX = point.X
					}
					if point.Y < minY {
						minY = point.Y
					}
					if point.Y > maxY {
						maxY = point.Y
					}
				}
			}
		}
	}

	if !hasPoints {
		return nil
	}

	// Calculate padding (percentage of content size, with minimum)
	contentWidth := maxX - minX
	contentHeight := maxY - minY
	paddingX := contentWidth * config.PaddingPercent
	paddingY := contentHeight * config.PaddingPercent

	if paddingX < config.MinPadding {
		paddingX = config.MinPadding
	}
	if paddingY < config.MinPadding {
		paddingY = config.MinPadding
	}

	return &boundingBox{
		minX:     minX,
		minY:     minY,
		maxX:     maxX,
		maxY:     maxY,
		paddingX: paddingX,
		paddingY: paddingY,
	}
}

// calculateImageDimensions calculates scale factors and image dimensions.
// The scale factor maintains aspect ratio while fitting content width into output width.
func calculateImageDimensions(bbox *boundingBox, config VisualizationConfig) (scaleX, scaleY float32, width, height int) {
	contentWidth := bbox.maxX - bbox.minX + bbox.paddingX*2
	contentHeight := bbox.maxY - bbox.minY + bbox.paddingY*2

	if contentWidth <= 0 {
		return 1, 1, config.OutputWidth, minImageHeight
	}

	// Scale to fit content width into output width, maintaining aspect ratio
	scaleX = float32(config.OutputWidth) / contentWidth
	scaleY = scaleX // Maintain aspect ratio
	height = int(contentHeight * scaleY)

	return scaleX, scaleY, config.OutputWidth, height
}

// drawStrokes draws all strokes onto the image with proper scaling.
// Highlighters are drawn first (background layer), then other strokes on top (foreground layer).
func drawStrokes(img *image.RGBA, pageData *rm.Rm, bbox *boundingBox, scaleX, scaleY float32, imgWidth, imgHeight int, config VisualizationConfig) {
	// First pass: draw all highlighters (background layer)
	drawStrokesByType(img, pageData, bbox, scaleX, scaleY, imgWidth, imgHeight, config, true)

	// Second pass: draw all other strokes (foreground layer)
	drawStrokesByType(img, pageData, bbox, scaleX, scaleY, imgWidth, imgHeight, config, false)
}

// drawStrokesByType draws strokes filtered by type (highlighters or non-highlighters).
func drawStrokesByType(img *image.RGBA, pageData *rm.Rm, bbox *boundingBox, scaleX, scaleY float32, imgWidth, imgHeight int, config VisualizationConfig, drawHighlighters bool) {
	for _, layer := range pageData.Layers {
		for _, line := range layer.Lines {
			if line.BrushType == rm.EraseArea || len(line.Points) < 2 {
				continue
			}

			isHighlighter := line.BrushType == rm.Highlighter || line.BrushType == rm.HighlighterV5
			if (drawHighlighters && !isHighlighter) || (!drawHighlighters && isHighlighter) {
				continue
			}

			pen := NewPenRenderer(line.BrushType, uint32(line.BrushColor), line.BrushSize)
			drawLine(img, line, bbox, scaleX, scaleY, imgWidth, imgHeight, pen, config)
		}
	}
}

// drawLine draws a single line with variable width, color, and opacity based on pen type.
// Highlighters are rendered using a special method for thick, semi-transparent background fills.
func drawLine(img *image.RGBA, line rm.Line, bbox *boundingBox, scaleX, scaleY float32, imgWidth, imgHeight int, pen *PenRenderer, config VisualizationConfig) {
	if len(line.Points) == 0 {
		return
	}

	// Highlighters use special rendering (thick, semi-transparent background fills)
	isHighlighter := line.BrushType == rm.Highlighter || line.BrushType == rm.HighlighterV5
	if isHighlighter {
		drawHighlighterLine(img, line, bbox, scaleX, scaleY, imgWidth, imgHeight, pen, config)
		return
	}

	// Regular strokes: draw with variable width, color, and opacity
	drawRegularStroke(img, line, bbox, scaleX, scaleY, imgWidth, imgHeight, pen, config)
}

// drawRegularStroke draws a regular stroke with variable width, color, and opacity.
func drawRegularStroke(img *image.RGBA, line rm.Line, bbox *boundingBox, scaleX, scaleY float32, imgWidth, imgHeight int, pen *PenRenderer, config VisualizationConfig) {
	// Draw first point
	p0 := line.Points[0]
	x0, y0 := transformPoint(p0.X, p0.Y, bbox, scaleX, scaleY)

	width0 := pen.GetStrokeWidth(p0.Speed, p0.Direction, p0.Width, p0.Pressure)
	color0 := pen.GetStrokeColor(p0.Speed, p0.Direction, p0.Width, p0.Pressure)
	opacity0 := pen.GetStrokeOpacity(p0.Speed, p0.Direction, p0.Width, p0.Pressure)

	radius0 := clampStrokeWidth(int(width0*config.StrokeWidthScale), config)
	strokeColor0 := color.RGBA{color0[0], color0[1], color0[2], uint8(255 * opacity0)}
	drawFilledCircle(img, x0, y0, radius0, strokeColor0, imgWidth, imgHeight)

	// Draw segments between points
	for i := 0; i < len(line.Points)-1; i++ {
		p1, p2 := line.Points[i], line.Points[i+1]
		x1, y1 := transformPoint(p1.X, p1.Y, bbox, scaleX, scaleY)
		x2, y2 := transformPoint(p2.X, p2.Y, bbox, scaleX, scaleY)

		width1 := pen.GetStrokeWidth(p1.Speed, p1.Direction, p1.Width, p1.Pressure)
		width2 := pen.GetStrokeWidth(p2.Speed, p2.Direction, p2.Width, p2.Pressure)
		color1 := pen.GetStrokeColor(p1.Speed, p1.Direction, p1.Width, p1.Pressure)
		color2 := pen.GetStrokeColor(p2.Speed, p2.Direction, p2.Width, p2.Pressure)
		opacity1 := pen.GetStrokeOpacity(p1.Speed, p1.Direction, p1.Width, p1.Pressure)
		opacity2 := pen.GetStrokeOpacity(p2.Speed, p2.Direction, p2.Width, p2.Pressure)

		pixelWidth1 := clampStrokeWidth(int(width1*config.StrokeWidthScale), config)
		pixelWidth2 := clampStrokeWidth(int(width2*config.StrokeWidthScale), config)

		drawVariableWidthLineWithColor(img, x1, y1, pixelWidth1, x2, y2, pixelWidth2,
			color1, color2, opacity1, opacity2, imgWidth, imgHeight)
	}
}

// drawHighlighterLine draws a highlighter line as thick, semi-transparent filled shapes.
// Highlighters color the background rather than drawing strokes on top.
func drawHighlighterLine(img *image.RGBA, line rm.Line, bbox *boundingBox, scaleX, scaleY float32, imgWidth, imgHeight int, pen *PenRenderer, config VisualizationConfig) {
	if len(line.Points) < 2 {
		return
	}

	// Lighten the color for highlighters (make it more pastel)
	lightColor := lightenColor(pen.baseColor)

	// Calculate highlighter width (thick stroke)
	width := calculateHighlighterWidth(config)

	// Convert all points to image coordinates
	points := make([]struct{ x, y int }, len(line.Points))
	for i, p := range line.Points {
		points[i].x, points[i].y = transformPoint(p.X, p.Y, bbox, scaleX, scaleY)
	}

	// Draw thick continuous stroke with alpha blending
	alphaValue := float32(255) * highlighterOpacity
	alpha := uint8(alphaValue)
	strokeColor := color.RGBA{lightColor[0], lightColor[1], lightColor[2], alpha}
	drawThickContinuousStroke(img, points, width, strokeColor, imgWidth, imgHeight)
}

// lightenColor lightens a color by mixing it with white for a pastel effect.
func lightenColor(baseColor [3]uint8) [3]uint8 {
	return [3]uint8{
		uint8(float32(baseColor[0])*highlighterColorLighten + 255*highlighterWhiteMix),
		uint8(float32(baseColor[1])*highlighterColorLighten + 255*highlighterWhiteMix),
		uint8(float32(baseColor[2])*highlighterColorLighten + 255*highlighterWhiteMix),
	}
}

// calculateHighlighterWidth calculates the width for highlighter strokes.
func calculateHighlighterWidth(config VisualizationConfig) int {
	width := int(highlighterBaseWidthPixels * config.StrokeWidthScale * highlighterWidthMultiplier)
	if width < highlighterMinWidth {
		width = highlighterMinWidth
	}
	if width > highlighterMaxWidth {
		width = highlighterMaxWidth
	}
	return width
}

// clampStrokeWidth clamps stroke width to configured min/max values.
func clampStrokeWidth(width int, config VisualizationConfig) int {
	if width < config.MinStrokeWidth {
		return config.MinStrokeWidth
	}
	if width > config.MaxStrokeWidth {
		return config.MaxStrokeWidth
	}
	return width
}

// transformPoint transforms a point from document coordinates to image coordinates.
func transformPoint(x, y float32, bbox *boundingBox, scaleX, scaleY float32) (int, int) {
	imgX := (x - bbox.minX + bbox.paddingX) * scaleX
	imgY := (y - bbox.minY + bbox.paddingY) * scaleY
	return int(imgX), int(imgY)
}

// Image creation and saving functions

// fillWhiteBackground fills the entire image with white.
func fillWhiteBackground(img *image.RGBA, width, height int) {
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}
}

// createEmptyImage creates an empty white image.
func createEmptyImage(outputPath string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillWhiteBackground(img, width, height)
	return savePNG(img, outputPath)
}

// savePNG saves an image as a PNG file.
func savePNG(img image.Image, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

// Drawing primitives

// drawFilledCircle draws a filled circle (opaque, no blending).
func drawFilledCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA, imgWidth, imgHeight int) {
	if radius <= 0 {
		return
	}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				px := cx + dx
				py := cy + dy
				if px >= 0 && px < imgWidth && py >= 0 && py < imgHeight {
					img.Set(px, py, c)
				}
			}
		}
	}
}

// drawFilledCircleBlended draws a filled circle with alpha blending.
func drawFilledCircleBlended(img *image.RGBA, cx, cy, radius int, c color.RGBA, imgWidth, imgHeight int) {
	if radius <= 0 {
		return
	}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				px := cx + dx
				py := cy + dy
				if px >= 0 && px < imgWidth && py >= 0 && py < imgHeight {
					existing := img.RGBAAt(px, py)
					blended := blendColors(existing, c)
					img.Set(px, py, blended)
				}
			}
		}
	}
}

// drawThickContinuousStroke draws a thick continuous stroke through multiple points.
// Used for highlighters to create smooth, thick strokes without visible dots.
func drawThickContinuousStroke(img *image.RGBA, points []struct{ x, y int }, width int, strokeColor color.RGBA, imgWidth, imgHeight int) {
	if len(points) < 2 {
		return
	}

	halfWidth := float32(width) / 2

	// Draw segments between consecutive points
	for i := 0; i < len(points)-1; i++ {
		p1, p2 := points[i], points[i+1]

		dx := float32(p2.x - p1.x)
		dy := float32(p2.y - p1.y)
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if length == 0 {
			continue
		}

		// Normalize direction
		dx /= length
		dy /= length

		// Perpendicular direction for width
		px := -dy
		py := dx

		// Calculate the four corners of the segment (rectangle)
		sx1 := float32(p1.x) + px*halfWidth
		sy1 := float32(p1.y) + py*halfWidth
		sx2 := float32(p1.x) - px*halfWidth
		sy2 := float32(p1.y) - py*halfWidth

		ex1 := float32(p2.x) + px*halfWidth
		ey1 := float32(p2.y) + py*halfWidth
		ex2 := float32(p2.x) - px*halfWidth
		ey2 := float32(p2.y) - py*halfWidth

		// Draw filled polygon (rectangle part) with alpha blending
		drawFilledPolygonBlended(img,
			[]int{int(sx1), int(sx2), int(ex2), int(ex1)},
			[]int{int(sy1), int(sy2), int(ey2), int(ey1)},
			strokeColor, imgWidth, imgHeight)

		// Draw rounded cap at end point (except for last segment)
		if i == len(points)-2 {
			drawFilledCircleBlended(img, p2.x, p2.y, width/2, strokeColor, imgWidth, imgHeight)
		}
	}

	// Draw rounded cap at start point
	drawFilledCircleBlended(img, points[0].x, points[0].y, width/2, strokeColor, imgWidth, imgHeight)
}

// drawFilledPolygonBlended draws a filled polygon with alpha blending.
func drawFilledPolygonBlended(img *image.RGBA, xs, ys []int, c color.RGBA, imgWidth, imgHeight int) {
	if len(xs) != len(ys) || len(xs) < 3 {
		return
	}

	// Find bounding box
	minX, maxX := xs[0], xs[0]
	minY, maxY := ys[0], ys[0]
	for i := 1; i < len(xs); i++ {
		if xs[i] < minX {
			minX = xs[i]
		}
		if xs[i] > maxX {
			maxX = xs[i]
		}
		if ys[i] < minY {
			minY = ys[i]
		}
		if ys[i] > maxY {
			maxY = ys[i]
		}
	}

	// Clamp to image bounds
	if minX < 0 {
		minX = 0
	}
	if maxX >= imgWidth {
		maxX = imgWidth - 1
	}
	if minY < 0 {
		minY = 0
	}
	if maxY >= imgHeight {
		maxY = imgHeight - 1
	}

	// Point-in-polygon test for each pixel in bounding box
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInPolygon(x, y, xs, ys) {
				if x >= 0 && x < imgWidth && y >= 0 && y < imgHeight {
					existing := img.RGBAAt(x, y)
					blended := blendColors(existing, c)
					img.Set(x, y, blended)
				}
			}
		}
	}
}

// drawVariableWidthLineWithColor draws a line with variable width, color, and opacity.
// Interpolates width, color, and opacity smoothly between start and end points.
func drawVariableWidthLineWithColor(img *image.RGBA, x1, y1 int, width1 int, x2, y2 int, width2 int,
	color1, color2 [3]uint8, opacity1, opacity2 float32, imgWidth, imgHeight int) {
	dx := float32(x2 - x1)
	dy := float32(y2 - y1)
	length := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	if length == 0 {
		c := color.RGBA{color1[0], color1[1], color1[2], uint8(255 * opacity1)}
		drawFilledCircle(img, x1, y1, width1, c, imgWidth, imgHeight)
		return
	}

	// Normalize direction
	dx /= length
	dy /= length

	// Draw line with variable width, color, and opacity
	steps := int(length) + 1
	if steps < 2 {
		steps = 2
	}

	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)

		// Interpolate position
		x := float32(x1) + dx*length*t
		y := float32(y1) + dy*length*t

		// Interpolate width
		width := float32(width1) + (float32(width2)-float32(width1))*t
		radius := int(width + 0.5)
		if radius < 1 {
			radius = 1
		}

		// Interpolate color
		r := uint8(float32(color1[0]) + (float32(color2[0])-float32(color1[0]))*t)
		g := uint8(float32(color1[1]) + (float32(color2[1])-float32(color1[1]))*t)
		b := uint8(float32(color1[2]) + (float32(color2[2])-float32(color1[2]))*t)

		// Interpolate opacity
		opacity := opacity1 + (opacity2-opacity1)*t
		a := uint8(255 * opacity)

		c := color.RGBA{r, g, b, a}
		drawFilledCircle(img, int(x+0.5), int(y+0.5), radius, c, imgWidth, imgHeight)
	}
}

// Utility functions

// pointInPolygon tests if a point is inside a polygon using ray casting algorithm.
func pointInPolygon(x, y int, xs, ys []int) bool {
	inside := false
	j := len(xs) - 1
	for i := 0; i < len(xs); i++ {
		if ((ys[i] > y) != (ys[j] > y)) &&
			(x < (xs[j]-xs[i])*(y-ys[i])/(ys[j]-ys[i])+xs[i]) {
			inside = !inside
		}
		j = i
	}
	return inside
}

// blendColors blends two colors using alpha compositing (over operator).
// Used for semi-transparent rendering (e.g., highlighters).
func blendColors(bg color.RGBA, fg color.RGBA) color.RGBA {
	alpha := float32(fg.A) / 255.0
	invAlpha := 1.0 - alpha

	r := uint8(float32(fg.R)*alpha + float32(bg.R)*invAlpha)
	g := uint8(float32(fg.G)*alpha + float32(bg.G)*invAlpha)
	b := uint8(float32(fg.B)*alpha + float32(bg.B)*invAlpha)

	return color.RGBA{r, g, b, 255}
}

