package rm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// UnmarshalBinary implements encoding.UnmarshalBinary for
// transforming bytes into a Rm page
func (rm *Rm) UnmarshalBinary(data []byte) error {
	r := newReader(data)
	if err := r.checkHeader(); err != nil {
		return err
	}
	rm.Version = r.version

	// V6 has a different format structure (tagged block format)
	if r.version == V6 {
		return unmarshalV6Tagged(rm, data)
	}

	nbLayers, err := r.readNumber()
	if err != nil {
		return err
	}

	rm.Layers = make([]Layer, nbLayers)
	for i := uint32(0); i < nbLayers; i++ {
		nbLines, err := r.readNumber()
		if err != nil {
			return err
		}

		rm.Layers[i].Lines = make([]Line, nbLines)
		for j := uint32(0); j < nbLines; j++ {
			line, err := r.readLine()
			if err != nil {
				return err
			}
			rm.Layers[i].Lines[j] = line
		}
	}

	return nil
}

type reader struct {
	bytes.Reader
	version Version
}

func newReader(data []byte) reader {
	br := bytes.NewReader(data)

	// we set V5 as default but the real value is
	// analysed when checking the header
	return reader{*br, V5}
}

func (r *reader) checkHeader() error {
	buf := make([]byte, HeaderLen)

	n, err := r.Read(buf)
	if err != nil {
		return err
	}

	if n != HeaderLen {
		return fmt.Errorf("Wrong header size")
	}

	switch string(buf) {
	case HeaderV5:
		r.version = V5
	case HeaderV3:
		r.version = V3
	case HeaderV6:
		r.version = V6
	default:
		// Check if it's V6 with different padding
		if strings.Contains(string(buf), "version=6") {
			r.version = V6
		} else {
			return fmt.Errorf("Unknown header")
		}
	}

	return nil
}

func (r *reader) readNumber() (uint32, error) {
	var nb uint32
	if err := binary.Read(r, binary.LittleEndian, &nb); err != nil {
		return 0, fmt.Errorf("Wrong number read")
	}
	return nb, nil
}

func (r *reader) readLine() (Line, error) {
	var line Line

	if err := binary.Read(r, binary.LittleEndian, &line.BrushType); err != nil {
		return line, fmt.Errorf("Failed to read line")
	}

	if err := binary.Read(r, binary.LittleEndian, &line.BrushColor); err != nil {
		return line, fmt.Errorf("Failed to read line")
	}

	if err := binary.Read(r, binary.LittleEndian, &line.Padding); err != nil {
		return line, fmt.Errorf("Failed to read line")
	}

	if err := binary.Read(r, binary.LittleEndian, &line.BrushSize); err != nil {
		return line, fmt.Errorf("Failed to read line")
	}

	// this new attribute has been added in v5
	if r.version == V5 {
		if err := binary.Read(r, binary.LittleEndian, &line.Unknown); err != nil {
			return line, fmt.Errorf("Failed to read line")
		}
	}

	nbPoints, err := r.readNumber()
	if err != nil {
		return line, err
	}

	if nbPoints == 0 {
		return line, nil
	}

	line.Points = make([]Point, nbPoints)

	for i := uint32(0); i < nbPoints; i++ {
		p, err := r.readPoint()
		if err != nil {
			return line, err
		}

		line.Points[i] = p
	}

	return line, nil
}

func (r *reader) readPoint() (Point, error) {
	var point Point

	if err := binary.Read(r, binary.LittleEndian, &point.X); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Y); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Speed); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Direction); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Width); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Pressure); err != nil {
		return point, fmt.Errorf("Failed to read point")
	}

	return point, nil
}

// unmarshalV6Tagged is the new V6 parser using tagged block format
// The implementation is in unmarshal_v6.go
func unmarshalV6Tagged(rm *Rm, data []byte) error {
	return unmarshalV6(rm, data)
}

// Old V6 parser - kept for reference but not used
// This was an incorrect heuristic-based parser
func unmarshalV6Old(rm *Rm, data []byte) error {
	if len(data) < 43 {
		return fmt.Errorf("file too short")
	}

	pos := 43 // Skip header

	// Skip initial metadata (5 bytes)
	if pos+5 > len(data) {
		return fmt.Errorf("unexpected end of file")
	}
	pos += 5

	// Skip flags (5 bytes)
	if pos+5 > len(data) {
		return fmt.Errorf("unexpected end of file")
	}
	pos += 5

	// Read layer count
	if pos+4 > len(data) {
		return fmt.Errorf("unexpected end of file")
	}
	numLayers := binary.LittleEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Skip UUID (16 bytes)
	if pos+16 > len(data) {
		return fmt.Errorf("unexpected end of file")
	}
	pos += 16

	// Skip more metadata (7 bytes)
	if pos+7 > len(data) {
		return fmt.Errorf("unexpected end of file")
	}
	pos += 7

	rm.Layers = make([]Layer, numLayers)

	// Parse each layer
	for layerIdx := uint32(0); layerIdx < numLayers; layerIdx++ {
		var lines []Line

		// Find "Layer N" string to mark layer start
		layerNamePos := -1
		for i := pos; i < len(data)-10; i++ {
			if i+7 < len(data) && string(data[i:i+7]) == "Layer " {
				layerNamePos = i
				break
			}
		}

		if layerNamePos < 0 {
			// No more layer markers found, stop parsing
			break
		}

		// Skip past "Layer N" marker and find where it ends
		// Format: "Layer N<" followed by metadata
		nameEnd := layerNamePos + 7 // After "Layer "
		// Skip layer number (could be 1-2 digits)
		for nameEnd < len(data) && nameEnd < layerNamePos+15 {
			if data[nameEnd] == '<' || data[nameEnd] == 0 {
				break
			}
			nameEnd++
		}
		// Skip the '<' or null terminator
		if nameEnd < len(data) && (data[nameEnd] == '<' || data[nameEnd] == 0) {
			nameEnd++
		}

		// Skip layer metadata after "Layer N<"
		// The metadata appears to be variable length. We need to find where actual stroke data starts.
		// Stroke data starts with a brush type (uint32, typically 0-20), followed by brush color, etc.
		// Let's search for a pattern that looks like a valid stroke header.
		parseStart := nameEnd
		parseEnd := len(data)
		
		// Find the next layer marker to determine end boundary
		if layerIdx < numLayers-1 {
			for i := parseStart + 20; i < len(data)-10; i++ {
				if i+7 < len(data) && string(data[i:i+7]) == "Layer " {
					parseEnd = i
					break
				}
			}
		}

		// Try to find where actual stroke data starts by looking for valid stroke headers
		// A valid stroke header has: brushType (0-20), brushColor, padding, brushSize (reasonable float)
		bestStart := parseStart
		for searchPos := parseStart; searchPos < parseStart+100 && searchPos < parseEnd-50; searchPos++ {
			if searchPos+28 > parseEnd {
				break
			}
			brushType := binary.LittleEndian.Uint32(data[searchPos : searchPos+4])
			if brushType > 20 {
				continue
			}
			brushSizeBits := binary.LittleEndian.Uint32(data[searchPos+12 : searchPos+16])
			brushSize := math.Float32frombits(brushSizeBits)
			if brushSize >= 0.1 && brushSize <= 50.0 {
				numPoints := binary.LittleEndian.Uint32(data[searchPos+20 : searchPos+24])
				if numPoints > 0 && numPoints < 10000 {
					// This looks like a valid stroke header
					bestStart = searchPos
					break
				}
			}
		}

		// Parse lines AFTER finding valid stroke data start
		linePos := bestStart
		for linePos < parseEnd-50 { // Need at least 50 bytes for a line with points
			savedPos := linePos

			// Try to read brush type (uint32)
			if linePos+4 > parseEnd {
				break
			}
			brushType := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			linePos += 4

			// Brush types are typically 0-20
			if brushType > 20 {
				linePos = savedPos + 1
				continue
			}

			// Try to read brush color
			if linePos+4 > parseEnd {
				break
			}
			brushColor := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			linePos += 4

			// Try to read padding
			if linePos+4 > parseEnd {
				break
			}
			padding := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			linePos += 4

			// Try to read brush size (float32)
			if linePos+4 > parseEnd {
				break
			}
			brushSizeBits := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			brushSize := math.Float32frombits(brushSizeBits)
			linePos += 4

			// Validate brush size is reasonable (typically 0.1 to 50.0)
			if brushSize < 0.01 || brushSize > 50.0 {
				linePos = savedPos + 1
				continue
			}

			// Try to read unknown field
			if linePos+4 > parseEnd {
				break
			}
			unknownBits := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			unknown := math.Float32frombits(unknownBits)
			linePos += 4

			// Try to read number of points
			if linePos+4 > parseEnd {
				break
			}
			numPoints := binary.LittleEndian.Uint32(data[linePos : linePos+4])
			linePos += 4

			// Validate numPoints is reasonable
			if numPoints == 0 || numPoints > 10000 {
				linePos = savedPos + 1
				continue
			}

			// Try to read points - each point is 24 bytes (X, Y, Speed, Direction, Width, Pressure)
			pointsNeeded := int(numPoints) * 24
			if linePos+pointsNeeded > parseEnd {
				linePos = savedPos + 1
				continue
			}

			// Successfully parsed line header, now read points
			line := Line{
				BrushType:  BrushType(brushType),
				BrushColor: BrushColor(brushColor),
				Padding:    padding,
				BrushSize:  BrushSize(brushSize),
				Unknown:    unknown,
				Points:     make([]Point, numPoints),
			}

			pointsRead := 0
			for i := uint32(0); i < numPoints; i++ {
				if linePos+24 > parseEnd {
					break
				}

				point := Point{}
				x := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4
				y := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4
				speed := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4
				direction := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4
				width := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4
				pressure := math.Float32frombits(binary.LittleEndian.Uint32(data[linePos : linePos+4]))
				linePos += 4

				// Validate values are not NaN or Inf, and are reasonable
				if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) ||
					math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) ||
					math.IsNaN(float64(speed)) || math.IsInf(float64(speed), 0) ||
					math.IsNaN(float64(direction)) || math.IsInf(float64(direction), 0) ||
					math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) ||
					math.IsNaN(float64(pressure)) || math.IsInf(float64(pressure), 0) {
					continue
				}

				// Also validate coordinates are reasonable (within page bounds)
				// Remarkable page is typically around 1872x1404 pixels, but coordinates can be larger
				// Filter out extremely small values that are likely metadata misinterpreted as coordinates
				if x < 0.1 || x > 5000 || y < 0.1 || y > 5000 {
					continue
				}
				
				// Validate pressure, width, speed are reasonable
				// Filter out extremely small or large values that are likely metadata
				if pressure < 0.001 || pressure > 1 || width < 0.01 || width > 100 || speed < 0 || speed > 10000 {
					continue
				}

				point.X = x
				point.Y = y
				point.Speed = speed
				point.Direction = direction
				point.Width = width
				point.Pressure = pressure

				line.Points[pointsRead] = point
				pointsRead++
			}

			// Resize points array to actual points read
			if pointsRead > 0 {
				line.Points = line.Points[:pointsRead]
			}

				// Only accept lines where ALL points have valid coordinates
				// This filters out metadata that's being misinterpreted as strokes
				if pointsRead >= 2 {
					// Filter out points with invalid coordinates
					validPoints := make([]Point, 0, pointsRead)
					for _, pt := range line.Points {
						// Only keep points with reasonable coordinates and values
						// Be very strict - coordinates must be in a reasonable range for Remarkable pages
						if pt.X >= 1.0 && pt.X <= 2000 && pt.Y >= 1.0 && pt.Y <= 2000 &&
							pt.Pressure >= 0.01 && pt.Pressure <= 1 &&
							pt.Width >= 0.1 && pt.Width <= 50 {
							validPoints = append(validPoints, pt)
						}
					}
					
					// Only accept lines where ALL points are valid (100%) AND we have at least 2 points
					// This ensures we're not parsing metadata
					if len(validPoints) == pointsRead && len(validPoints) >= 2 {
						line.Points = validPoints
						lines = append(lines, line)
					} else {
						linePos = savedPos + 1
						continue
					}
				} else {
					linePos = savedPos + 1
					continue
				}
		}

		rm.Layers[layerIdx].Lines = lines

		// Move to position after this layer's data for next iteration
		pos = parseEnd
	}

	return nil
}
