package rm

import (
	"encoding/binary"
	"fmt"
	"math"
)

// V6 uses a tagged block format. This implementation is based on rmscene's parser.

const (
	blockTypeSceneLineItem = 0x05
	pointSizeV2            = 14 // Version 2: float32 x,y (8) + uint16 speed,width (4) + uint8 direction,pressure (2)
)

// Tag types
const (
	tagTypeByte1   = 0x1
	tagTypeByte4   = 0x4
	tagTypeByte8   = 0x8
	tagTypeLength4 = 0xC
	tagTypeID      = 0xF
)

// dataStream wraps a byte slice with position tracking
type dataStream struct {
	data []byte
	pos  int
}

func newDataStream(data []byte) *dataStream {
	return &dataStream{data: data, pos: 0}
}

func (ds *dataStream) tell() int {
	return ds.pos
}

func (ds *dataStream) readBytes(n int) ([]byte, error) {
	if ds.pos+n > len(ds.data) {
		return nil, fmt.Errorf("unexpected end of data")
	}
	result := ds.data[ds.pos : ds.pos+n]
	ds.pos += n
	return result, nil
}

func (ds *dataStream) readVaruint() (uint64, error) {
	shift := 0
	result := uint64(0)
	for {
		if ds.pos >= len(ds.data) {
			return 0, fmt.Errorf("unexpected end of data")
		}
		b := ds.data[ds.pos]
		ds.pos++
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("varuint too large")
		}
	}
	return result, nil
}

func (ds *dataStream) readUint8() (uint8, error) {
	b, err := ds.readBytes(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (ds *dataStream) readUint16() (uint16, error) {
	b, err := ds.readBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

func (ds *dataStream) readUint32() (uint32, error) {
	b, err := ds.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (ds *dataStream) readFloat32() (float32, error) {
	b, err := ds.readBytes(4)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(b)), nil
}

func (ds *dataStream) readFloat64() (float64, error) {
	b, err := ds.readBytes(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
}

func (ds *dataStream) readCrdtID() error {
	// CRDT ID: uint8 part1 + varuint part2
	_, err := ds.readUint8()
	if err != nil {
		return err
	}
	_, err = ds.readVaruint()
	return err
}

func (ds *dataStream) readTag() (int, int, error) {
	// Tag format: varuint where (index << 4) | tagType
	tagValue, err := ds.readVaruint()
	if err != nil {
		return 0, 0, err
	}
	index := int(tagValue >> 4)
	tagType := int(tagValue & 0xF)
	return index, tagType, nil
}

func (ds *dataStream) checkTag(expectedIndex, expectedType int) bool {
	savedPos := ds.pos
	index, tagType, err := ds.readTag()
	if err != nil {
		ds.pos = savedPos
		return false
	}
	ds.pos = savedPos
	return index == expectedIndex && tagType == expectedType
}

func (ds *dataStream) readTaggedValue(expectedIndex, expectedType int) error {
	index, tagType, err := ds.readTag()
	if err != nil {
		return err
	}
	if index != expectedIndex || tagType != expectedType {
		return fmt.Errorf("unexpected tag: expected index=%d type=%d, got index=%d type=%d", expectedIndex, expectedType, index, tagType)
	}
	return nil
}

func (ds *dataStream) bytesRemaining() int {
	return len(ds.data) - ds.pos
}

// unmarshalV6 parses version 6 .rm files using the tagged block format
func unmarshalV6(rm *Rm, data []byte) error {
	if len(data) < 43 {
		return fmt.Errorf("file too short")
	}

	pos := 43 // Skip header
	var allLines []Line
	blockCount := 0
	lineBlockCount := 0

	// Parse blocks sequentially
	for pos < len(data)-8 {
		// Read block header: uint32 length, uint8 unknown, uint8 min_version, uint8 current_version, uint8 block_type
		if pos+8 > len(data) {
			break
		}

		blockLength := binary.LittleEndian.Uint32(data[pos : pos+4])
		pos += 4

		unknown := data[pos]
		pos++
		if unknown != 0 {
			// Skip blocks with non-zero unknown field
			// blockLength is the size of data after the 8-byte header
			// We've already read 4 bytes (unknown, min_v, cur_v, block_type), so skip blockLength
			if int(blockLength) > len(data)-pos {
				break
			}
			pos += int(blockLength)
			continue
		}

		_ = data[pos] // minVersion (unused)
		pos++
		currentVersion := data[pos]
		pos++
		blockType := data[pos]
		pos++

		blockStart := pos
		// blockLength is the size of data after the 8-byte header
		blockEnd := blockStart + int(blockLength)

		if blockEnd > len(data) {
			break
		}

		blockCount++
		// Only process SceneLineItemBlock blocks (type 0x05)
		if blockType == blockTypeSceneLineItem {
			lineBlockCount++
			blockData := data[blockStart:blockEnd]
			lines, _ := parseSceneLineItemBlock(blockData, currentVersion)
			// Include lines even if there was an error, as long as we got some lines
			if len(lines) > 0 {
				allLines = append(allLines, lines...)
			}
		}

		pos = blockEnd
	}

	// Group lines into layers (for now, put all lines in one layer)
	if len(allLines) > 0 {
		rm.Layers = []Layer{{Lines: allLines}}
	} else {
		rm.Layers = []Layer{}
	}

	return nil
}

// parseSceneLineItemBlock extracts lines from a SceneLineItemBlock
func parseSceneLineItemBlock(blockData []byte, version uint8) ([]Line, error) {
	ds := newDataStream(blockData)
	var lines []Line

	// SceneLineItemBlock structure:
	// - parent_id (tagged ID, index=1)
	// - item_id (tagged ID, index=2)
	// - left_id (tagged ID, index=3)
	// - right_id (tagged ID, index=4)
	// - deleted_length (tagged int, index=5)
	// - subblock(6) containing line data

	// Skip tags 1-5 (parent_id, item_id, left_id, right_id, deleted_length)
	// These are the SceneItemBlock header tags, not the line data tags
	for i := 0; i < 5 && ds.bytesRemaining() > 0; i++ {
		savedPos := ds.pos
		_, tagType, err := ds.readTag()
		if err != nil {
			break
		}

		// Skip the value based on tag type
		switch tagType {
		case tagTypeID:
			_ = ds.readCrdtID()
		case tagTypeByte4:
			_, _ = ds.readUint32()
		default:
			// Unknown type, skip
			if ds.bytesRemaining() > 0 {
				ds.pos = savedPos + 1
			}
		}
	}

	// Look for subblock(6) - tag index=6, type=Length4
	for ds.bytesRemaining() > 5 {
		savedPos := ds.pos
		index, tagType, err := ds.readTag()
		if err != nil {
			break
		}

		if index == 6 && tagType == tagTypeLength4 {
			// Found subblock(6)
			subblockLength, err := ds.readUint32()
			if err != nil {
				ds.pos = savedPos + 1
				continue
			}

			if int(subblockLength) > ds.bytesRemaining() {
				ds.pos = savedPos + 1
				continue
			}

			subblockStart := ds.pos
			subblockData := blockData[subblockStart : subblockStart+int(subblockLength)]

			// Parse line from subblock
			line, err := parseLineFromSubblock(subblockData, version)
			// Include line if it has valid points (at least 2 points with valid coordinates)
			if len(line.Points) >= 2 && hasValidPoints(line.Points) {
				lines = append(lines, line)
			}

			ds.pos += int(subblockLength)
			// Continue processing in case there are multiple subblock(6) entries
			// (though typically there's only one per SceneLineItemBlock)
		} else {
			// Not the tag we're looking for, skip this tag's value
			switch tagType {
			case tagTypeByte1:
				_, _ = ds.readUint8()
			case tagTypeByte4:
				_, _ = ds.readUint32()
			case tagTypeByte8:
				_, _ = ds.readFloat64()
			case tagTypeLength4:
				length, _ := ds.readUint32()
				if int(length) <= ds.bytesRemaining() {
					ds.pos += int(length)
				} else {
					ds.pos = savedPos + 1
				}
			case tagTypeID:
				_ = ds.readCrdtID()
			default:
				ds.pos = savedPos + 1
			}
		}
	}

	return lines, nil
}

// parseLineFromSubblock extracts a Line from a subblock containing line data
// The subblock starts with item_type (uint8, should be 0x03), then the line data
func parseLineFromSubblock(subblockData []byte, version uint8) (Line, error) {
	ds := newDataStream(subblockData)
	line := Line{}

	// Read item_type (uint8, should be 0x03 for SceneLineItemBlock)
	if ds.bytesRemaining() < 1 {
		return line, fmt.Errorf("subblock too short for item_type")
	}
	itemType, err := ds.readUint8()
	if err != nil {
		return line, err
	}
	if itemType != 0x03 {
		return line, fmt.Errorf("unexpected item_type: %d", itemType)
	}

	// Parse tagged fields:
	// - tag 1: tool_id (int/Byte4)
	// - tag 2: color_id (int/Byte4)
	// - tag 3: thickness_scale (double/Byte8)
	// - tag 4: starting_length (float/Byte4)
	// - tag 5: subblock with points (Length4) - this is the critical one
	// - tag 6: timestamp (ID)
	// - tag 7: move_id (ID, optional)
	// - tag 8: unknown float (optional)

	var toolID, colorID uint32
	var thicknessScale float64
	var startingLength float32
	var pointsData []byte

	// Read tags sequentially to find tag 5 (points subblock) and other tags
	ds.pos = 1 // After item_type
	for ds.bytesRemaining() > 0 {
		savedPos := ds.pos
		index, tagType, err := ds.readTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == tagTypeByte4:
			val, _ := ds.readUint32()
			if toolID == 0 {
				toolID = val
			}
		case index == 2 && tagType == tagTypeByte4:
			val, _ := ds.readUint32()
			if colorID == 0 {
				colorID = val
			}
		case index == 3 && tagType == tagTypeByte8:
			val, _ := ds.readFloat64()
			if thicknessScale == 0 {
				thicknessScale = val
			}
		case index == 4 && tagType == tagTypeByte4:
			val, _ := ds.readFloat32()
			if startingLength == 0 {
				startingLength = val
			}
		case index == 5 && tagType == tagTypeLength4:
			// Points subblock - already handled above, but read it here too if not found
			if len(pointsData) == 0 {
				length, err := ds.readUint32()
				if err == nil && int(length) > 0 {
					pointsStart := ds.pos
					pointsEnd := pointsStart + int(length)
					if pointsEnd <= len(subblockData) {
						pointsData = make([]byte, int(length))
						copy(pointsData, subblockData[pointsStart:pointsEnd])
						ds.pos = pointsEnd
					}
				}
			} else {
				// Skip it
				length, _ := ds.readUint32()
				if int(length) <= ds.bytesRemaining() {
					ds.pos += int(length)
				}
			}
		default:
			// Skip unknown tags
			switch tagType {
			case tagTypeByte1:
				_, _ = ds.readUint8()
			case tagTypeByte4:
				_, _ = ds.readUint32()
			case tagTypeByte8:
				_, _ = ds.readFloat64()
			case tagTypeLength4:
				length, _ := ds.readUint32()
				if int(length) <= ds.bytesRemaining() {
					ds.pos += int(length)
				} else {
					ds.pos = savedPos + 1
				}
			case tagTypeID:
				_ = ds.readCrdtID()
			default:
				ds.pos = savedPos + 1
			}
		}
	}

	// Parse points if we found points data
	if len(pointsData) > 0 {
		points, err := parsePointsV2(pointsData)
		if err == nil && len(points) > 0 {
			line.Points = points
		}
	}

	if len(line.Points) == 0 {
		return line, fmt.Errorf("no valid points found")
	}

	line.BrushType = BrushType(toolID)
	line.BrushColor = BrushColor(colorID)
	line.BrushSize = BrushSize(float32(thicknessScale))
	line.Unknown = startingLength

	return line, nil
}

// parsePointsV2 parses points in version 2 format (14 bytes per point)
func parsePointsV2(data []byte) ([]Point, error) {
	if len(data)%pointSizeV2 != 0 {
		return nil, fmt.Errorf("points data length not multiple of %d", pointSizeV2)
	}

	numPoints := len(data) / pointSizeV2
	points := make([]Point, 0, numPoints)

	for i := 0; i < numPoints; i++ {
		offset := i * pointSizeV2
		if offset+pointSizeV2 > len(data) {
			break
		}

		pointData := data[offset : offset+pointSizeV2]

		// Version 2 format:
		// float32 x (4 bytes)
		// float32 y (4 bytes)
		// uint16 speed (2 bytes)
		// uint16 width (2 bytes)
		// uint8 direction (1 byte)
		// uint8 pressure (1 byte)

		x := math.Float32frombits(binary.LittleEndian.Uint32(pointData[0:4]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(pointData[4:8]))
		speed := float32(binary.LittleEndian.Uint16(pointData[8:10]))
		width := float32(binary.LittleEndian.Uint16(pointData[10:12]))
		direction := float32(pointData[12])
		pressure := float32(pointData[13]) / 255.0

		// Validate point - only filter out NaN and Inf values
		// Pages can have arbitrary dimensions, so we don't filter by coordinate ranges
		if !isValidCoordinate(x) || !isValidCoordinate(y) {
			continue
		}

		points = append(points, Point{
			X:         x,
			Y:         y,
			Speed:     speed,
			Width:     width,
			Direction: direction,
			Pressure:  pressure,
		})
	}

	return points, nil
}

// isValidCoordinate checks if a coordinate is valid (not NaN or Inf)
func isValidCoordinate(coord float32) bool {
	return !math.IsNaN(float64(coord)) && !math.IsInf(float64(coord), 0)
}

// hasValidPoints checks if all points in a slice have valid coordinates
func hasValidPoints(points []Point) bool {
	for _, pt := range points {
		if !isValidCoordinate(pt.X) || !isValidCoordinate(pt.Y) {
			return false
		}
	}
	return true
}
