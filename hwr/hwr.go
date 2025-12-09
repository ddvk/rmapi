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
			continue
		}
		text := extractTextFromResponse(c, output)
		textContent[pageNum] = text
	}

	return textContent, nil
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

			// Create stroke
			stroke := Stroke{
				X:           make([]float32, 0, len(line.Points)),
				Y:           make([]float32, 0, len(line.Points)),
				P:           make([]float32, 0, len(line.Points)),
				T:           make([]int64, 0, len(line.Points)),
				PointerType: pointerType,
			}

			timestamp := int64(0)
			for _, point := range line.Points {
				stroke.X = append(stroke.X, point.X)
				stroke.Y = append(stroke.Y, point.Y)

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
				stroke.P = append(stroke.P, pressure)

				// Add timestamp (increment by 16ms per point)
				stroke.T = append(stroke.T, timestamp)
				timestamp += 16
			}

			if len(stroke.X) > 0 && len(stroke.Y) > 0 {
				sg.Strokes = append(sg.Strokes, &stroke)
			}
		}
	}

	return json.Marshal(batch)
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
		return ""
	}

	data = bytes.TrimSpace(data)

	// Check if response is JSON (Jiix format)
	if len(data) > 0 && (data[0] == '{' || data[0] == '[') {
		text := extractTextFromJiix(data)
		if text != string(data) {
			return text
		}
	}

	// If it's supposed to be plain text, return as-is
	if expectedMimeType == "text/plain" {
		return string(data)
	}

	return string(data)
}

// extractTextFromJiix extracts text from Jiix JSON format
func extractTextFromJiix(data []byte) string {
	var jiix map[string]interface{}
	if err := json.Unmarshal(data, &jiix); err != nil {
		return string(data)
	}

	// Try to extract from "text" field
	if textField, ok := jiix["text"].(string); ok && textField != "" {
		return textField
	}

	// Try to extract from "label" field
	if label, ok := jiix["label"].(string); ok && label != "" {
		return label
	}

	// Try to extract from "words" array
	if words, ok := jiix["words"].([]interface{}); ok {
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
			return strings.Join(textParts, " ")
		}
	}

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

