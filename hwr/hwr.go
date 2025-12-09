package hwr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sync/semaphore"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/encoding/rm"
	"github.com/juruen/rmapi/log"
)

var NoContent = errors.New("no page content")

// Config holds HWR configuration
type Config struct {
	Page       int
	Lang       string
	InputType  string
	OutputFile string
	SplitPages bool
	BatchSize  int64
}

// Hwr processes an archive and performs handwriting recognition
func Hwr(zip *archive.Zip, cfg Config, applicationKey, hmacKey string) ([]string, error) {
	if applicationKey == "" {
		return nil, fmt.Errorf("RMAPI_HWR_APPLICATIONKEY environment variable is required")
	}
	if hmacKey == "" {
		return nil, fmt.Errorf("RMAPI_HWR_HMAC environment variable is required")
	}

	start := 0
	var end int

	if cfg.Page == 0 {
		start = zip.Content.LastOpenedPage
		end = start
	} else if cfg.Page < 0 {
		end = len(zip.Pages) - 1
	} else {
		start = cfg.Page - 1
		end = start
	}

	result := make([][]byte, len(zip.Pages))
	contenttype, output := setContentType(cfg.InputType)

	ctx := context.TODO()
	sem := semaphore.NewWeighted(cfg.BatchSize)
	for p := start; p <= end; p++ {
		if err := sem.Acquire(ctx, 1); err != nil {
			log.Trace.Printf("Failed to acquire semaphore: %v", err)
			break
		}
		go func(p int) {
			defer sem.Release(1)
			js, err := getJSON(zip, contenttype, cfg.Lang, p)
			if err != nil {
				log.Trace.Printf("Can't get page %d: %v", p, err)
				return
			}

			body, err := SendRequest(applicationKey, hmacKey, js, output)
			if err != nil {
				log.Trace.Printf("Failed to send request for page %d: %v", p, err)
				return
			}

			// Debug: Log response details
			if len(body) > 0 {
				previewLen := 200
				if len(body) < previewLen {
					previewLen = len(body)
				}
				log.Trace.Printf("Page %d: Received response (%d bytes), preview: %q", p, len(body), string(body[:previewLen]))
			} else {
				log.Trace.Printf("Page %d: Received empty response!", p)
			}

			result[p] = body
		}(p)
	}

	// Wait for all goroutines to finish
	if err := sem.Acquire(ctx, cfg.BatchSize); err != nil {
		log.Trace.Printf("Failed to acquire semaphore: %v", err)
	}

	var outputFiles []string

	if cfg.SplitPages {
		// Create separate file for each page
		for pageNum, c := range result {
			if c == nil || len(c) == 0 {
				continue
			}
			outputFile := fmt.Sprintf("%s_page_%d.txt", cfg.OutputFile, pageNum)
			if err := writeTextFile(outputFile, c, output); err != nil {
				log.Trace.Printf("Error writing file %s: %v", outputFile, err)
				continue
			}
			outputFiles = append(outputFiles, outputFile)
		}
	} else {
		// Single text file with all pages
		outputFile := cfg.OutputFile + ".txt"
		f, err := os.Create(outputFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file: %w", err)
		}

		for pageNum, c := range result {
			if c == nil || len(c) == 0 {
				continue
			}
			f.WriteString(fmt.Sprintf("=== Page %d ===\n", pageNum))
			text := extractTextFromResponse(c, output)
			f.WriteString(text)
			f.WriteString("\n\n")
		}
		f.Close()
		outputFiles = append(outputFiles, outputFile)
	}

	return outputFiles, nil
}

// HwrInline processes an archive and returns text content in memory (for inline mode)
func HwrInline(zip *archive.Zip, cfg Config, applicationKey, hmacKey string) (map[int]string, error) {
	if applicationKey == "" {
		return nil, fmt.Errorf("RMAPI_HWR_APPLICATIONKEY environment variable is required")
	}
	if hmacKey == "" {
		return nil, fmt.Errorf("RMAPI_HWR_HMAC environment variable is required")
	}

	start := 0
	var end int

	if cfg.Page == 0 {
		start = zip.Content.LastOpenedPage
		end = start
	} else if cfg.Page < 0 {
		end = len(zip.Pages) - 1
	} else {
		start = cfg.Page - 1
		end = start
	}

	result := make([][]byte, len(zip.Pages))
	contenttype, output := setContentType(cfg.InputType)

	ctx := context.TODO()
	sem := semaphore.NewWeighted(cfg.BatchSize)
	for p := start; p <= end; p++ {
		if err := sem.Acquire(ctx, 1); err != nil {
			log.Trace.Printf("Failed to acquire semaphore: %v", err)
			break
		}
		go func(p int) {
			defer sem.Release(1)
			js, err := getJSON(zip, contenttype, cfg.Lang, p)
			if err != nil {
				log.Trace.Printf("Can't get page %d: %v", p, err)
				return
			}

			body, err := SendRequest(applicationKey, hmacKey, js, output)
			if err != nil {
				log.Trace.Printf("Failed to send request for page %d: %v", p, err)
				return
			}

			// Debug: Log response details
			if len(body) > 0 {
				previewLen := 200
				if len(body) < previewLen {
					previewLen = len(body)
				}
				log.Trace.Printf("Page %d: Received response (%d bytes), preview: %q", p, len(body), string(body[:previewLen]))
			} else {
				log.Trace.Printf("Page %d: Received empty response!", p)
			}

			result[p] = body
		}(p)
	}

	// Wait for all goroutines to finish
	if err := sem.Acquire(ctx, cfg.BatchSize); err != nil {
		log.Trace.Printf("Failed to acquire semaphore: %v", err)
	}

	// Extract text content for each page
	textContent := make(map[int]string)
	for pageNum, c := range result {
		if c == nil || len(c) == 0 {
			log.Trace.Printf("Page %d: Skipping empty result", pageNum)
			continue
		}
		text := extractTextFromResponse(c, output)
		log.Trace.Printf("Page %d: Extracted text length: %d", pageNum, len(text))
		textContent[pageNum] = text
	}

	return textContent, nil
}

// downsamplePoints reduces the number of points in a stroke to reduce payload size
// Uses adaptive sampling: keeps every Nth point based on stroke length
func downsamplePoints(points []rm.Point) []rm.Point {
	if len(points) <= 2 {
		return points // Keep all points for very short strokes
	}

	// Determine sampling rate based on stroke length
	// Longer strokes need more aggressive downsampling to stay under 4MB limit
	sampleRate := 1
	if len(points) > 2000 {
		sampleRate = 6 // Very aggressive for extremely long strokes
	} else if len(points) > 1000 {
		sampleRate = 4 // Keep every 4th point for very long strokes
	} else if len(points) > 500 {
		sampleRate = 3 // Keep every 3rd point for long strokes
	} else if len(points) > 200 {
		sampleRate = 2 // Keep every 2nd point for medium strokes
	}

	// Always keep first and last points
	result := make([]rm.Point, 0, len(points)/sampleRate+2)
	result = append(result, points[0]) // First point

	// Sample middle points
	for i := sampleRate; i < len(points)-1; i += sampleRate {
		result = append(result, points[i])
	}

	// Always keep last point if different from first
	if len(points) > 1 {
		lastIdx := len(points) - 1
		if lastIdx > 0 && (points[lastIdx].X != points[0].X || points[lastIdx].Y != points[0].Y) {
			result = append(result, points[lastIdx])
		}
	}

	return result
}

// roundFloat32 rounds a float32 to the specified number of decimal places
func roundFloat32(val float32, decimals int) float32 {
	multiplier := float32(1)
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float32(int(val*multiplier+0.5)) / multiplier
}

// getJSON converts a page to MyScript API JSON format
func getJSON(zip *archive.Zip, contenttype string, lang string, pageNumber int) ([]byte, error) {
	numPages := len(zip.Pages)

	if pageNumber >= numPages || pageNumber < 0 {
		return nil, fmt.Errorf("page %d outside range, max: %d", pageNumber, numPages)
	}

	batch := BatchInput{
		Configuration: &Configuration{
			Lang: lang,
		},
		StrokeGroups: []*StrokeGroup{
			{},
		},
		ContentType: &contenttype,
		Width:       1404, // Remarkable2 screen width in pixels
		Height:      1872, // Remarkable2 screen height in pixels
		XDPI:        226,  // Remarkable2 DPI
		YDPI:        226,  // Remarkable2 DPI
	}

	sg := batch.StrokeGroups[0]
	page := zip.Pages[pageNumber]

	if page.Data == nil {
		return nil, NoContent
	}

	log.Trace.Printf("getJSON: Page %d has %d layers", pageNumber, len(page.Data.Layers))
	totalStrokes := 0
	totalPoints := 0

	for _, layer := range page.Data.Layers {
		for _, line := range layer.Lines {
			// Skip erase area strokes
			if line.BrushType == rm.EraseArea {
				continue
			}

			// Skip empty lines
			if len(line.Points) == 0 {
				continue
			}

			// Set pointer type - default to PEN, ERASER for eraser strokes
			pointerType := "PEN"
			if line.BrushType == rm.Eraser {
				pointerType = "ERASER"
			}

			// Downsample points to reduce payload size
			// Keep every Nth point, but always keep first and last
			downsampledPoints := downsamplePoints(line.Points)
			
			// Create stroke with downsampled points
			// Only include timestamps if stroke is short (they're optional and add significant size)
			includeTimestamps := len(downsampledPoints) < 100
			
			stroke := Stroke{
				X:           make([]float32, 0, len(downsampledPoints)),
				Y:           make([]float32, 0, len(downsampledPoints)),
				P:           make([]float32, 0, len(downsampledPoints)),
				PointerType: pointerType,
			}
			
			if includeTimestamps {
				stroke.T = make([]int64, 0, len(downsampledPoints))
			}

			timestamp := int64(0)
			for _, point := range downsampledPoints {
				// Reduce precision: round to 1 decimal place to reduce JSON size
				stroke.X = append(stroke.X, roundFloat32(point.X, 1))
				stroke.Y = append(stroke.Y, roundFloat32(point.Y, 1))

				// Normalize pressure
				pressure := point.Pressure
				if pressure <= 0 {
					pressure = 0.5
				} else if pressure > 1.0 {
					pressure = pressure / 10.0
					if pressure > 1.0 {
						pressure = 1.0
					}
				}
				// Round pressure to 2 decimal places
				stroke.P = append(stroke.P, roundFloat32(pressure, 2))

				// Add timestamp only for short strokes (optional field)
				if includeTimestamps {
					stroke.T = append(stroke.T, timestamp)
					timestamp += 16
				}
			}

			if len(stroke.X) > 0 && len(stroke.Y) > 0 {
				sg.Strokes = append(sg.Strokes, &stroke)
				totalStrokes++
				totalPoints += len(stroke.X)
			}
		}
	}

	log.Trace.Printf("getJSON: Page %d - Created %d strokes with %d total points (after downsampling)", pageNumber, totalStrokes, totalPoints)
	if totalStrokes == 0 {
		log.Trace.Printf("getJSON: WARNING - Page %d has no strokes to send to API!", pageNumber)
	}

	jsonData, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}

	// Log payload size
	sizeMB := float64(len(jsonData)) / (1024 * 1024)
	log.Trace.Printf("getJSON: Page %d - Payload size: %.2f MB (%d bytes)", pageNumber, sizeMB, len(jsonData))
	
	// If still too large, warn but don't fail (let API handle it)
	if len(jsonData) > 4000000 {
		log.Trace.Printf("getJSON: WARNING - Page %d payload still exceeds 4MB limit!", pageNumber)
	}

	return jsonData, nil
}

// setContentType maps input type to MyScript content type and output MIME type
func setContentType(requested string) (contenttype string, output string) {
	switch strings.ToLower(requested) {
	case "math":
		return "Math", "application/x-latex"
	case "text":
		return "Text", "text/plain"
	case "diagram":
		return "Diagram", "image/svg+xml"
	default:
		return "Text", "text/plain"
	}
}

// extractTextFromResponse extracts text from HWR API response
func extractTextFromResponse(data []byte, expectedMimeType string) string {
	if len(data) == 0 {
		log.Trace.Printf("extractTextFromResponse: empty data")
		return ""
	}

	data = bytes.TrimSpace(data)

	// Check if response is JSON (Jiix format)
	if len(data) > 0 && (data[0] == '{' || data[0] == '[') {
		text := extractTextFromJiix(data)
		if text != string(data) {
			log.Trace.Printf("extractTextFromResponse: extracted text from Jiix (%d chars)", len(text))
			return text
		}
		log.Trace.Printf("extractTextFromResponse: Jiix extraction returned original data")
	}

	// If it's supposed to be plain text, return as-is
	if expectedMimeType == "text/plain" {
		log.Trace.Printf("extractTextFromResponse: returning as plain text (%d chars)", len(data))
		return string(data)
	}

	log.Trace.Printf("extractTextFromResponse: returning raw data (%d chars)", len(data))
	return string(data)
}

// extractTextFromJiix extracts text from Jiix JSON format
func extractTextFromJiix(data []byte) string {
	var jiix map[string]interface{}
	if err := json.Unmarshal(data, &jiix); err != nil {
		log.Trace.Printf("extractTextFromJiix: failed to unmarshal JSON: %v", err)
		return string(data)
	}

	// Debug: Log available keys
	keys := make([]string, 0, len(jiix))
	for k := range jiix {
		keys = append(keys, k)
	}
	log.Trace.Printf("extractTextFromJiix: JSON keys available: %v", keys)

	// Try to extract from "text" field
	if textField, ok := jiix["text"].(string); ok && textField != "" {
		log.Trace.Printf("extractTextFromJiix: found text field (%d chars)", len(textField))
		return textField
	}

	// Try to extract from "label" field
	if label, ok := jiix["label"].(string); ok && label != "" {
		log.Trace.Printf("extractTextFromJiix: found label field (%d chars)", len(label))
		return label
	}

	// Try to extract from "words" array
	if words, ok := jiix["words"].([]interface{}); ok {
		log.Trace.Printf("extractTextFromJiix: found words array (%d words)", len(words))
		var textParts []string
		for _, word := range words {
			if wordMap, ok := word.(map[string]interface{}); ok {
				if label, ok := wordMap["label"].(string); ok && label != "" {
					textParts = append(textParts, label)
				} else if text, ok := wordMap["text"].(string); ok && text != "" {
					textParts = append(textParts, text)
				}
			}
		}
		if len(textParts) > 0 {
			log.Trace.Printf("extractTextFromJiix: extracted %d words", len(textParts))
			return strings.Join(textParts, " ")
		}
		log.Trace.Printf("extractTextFromJiix: words array found but no text extracted")
	}

	// Try to extract from "chars" array (character-level recognition)
	if chars, ok := jiix["chars"].([]interface{}); ok {
		log.Trace.Printf("extractTextFromJiix: found chars array (%d chars)", len(chars))
		var textParts []string
		for _, char := range chars {
			if charMap, ok := char.(map[string]interface{}); ok {
				if label, ok := charMap["label"].(string); ok && label != "" {
					textParts = append(textParts, label)
				} else if text, ok := charMap["text"].(string); ok && text != "" {
					textParts = append(textParts, text)
				}
			}
		}
		if len(textParts) > 0 {
			log.Trace.Printf("extractTextFromJiix: extracted %d chars", len(textParts))
			return strings.Join(textParts, "")
		}
	}

	log.Trace.Printf("extractTextFromJiix: no text found in Jiix format, returning raw data")
	return string(data)
}

// writeTextFile writes text content to a file
func writeTextFile(filename string, data []byte, mimeType string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	text := extractTextFromResponse(data, mimeType)
	_, err = f.WriteString(text)
	return err
}

