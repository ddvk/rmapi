package hwr

// Simplified models for MyScript API (without swagger dependencies)

// BatchInput represents the batch input for MyScript API
type BatchInput struct {
	Configuration *Configuration `json:"configuration,omitempty"`
	ContentType   *string        `json:"contentType"`
	StrokeGroups  []*StrokeGroup `json:"strokeGroups"`
	Width         int32          `json:"width,omitempty"`
	Height        int32          `json:"height,omitempty"`
	XDPI          float32        `json:"xDPI,omitempty"`
	YDPI          float32        `json:"yDPI,omitempty"`
}

// Configuration represents recognition configuration
type Configuration struct {
	Lang string `json:"lang,omitempty"`
}

// StrokeGroup represents a group of strokes
type StrokeGroup struct {
	Strokes []*Stroke `json:"strokes"`
}

// Stroke represents a single stroke
type Stroke struct {
	X           []float32 `json:"x"`
	Y           []float32 `json:"y"`
	P           []float32 `json:"p,omitempty"` // Pressure
	T           []int64   `json:"t,omitempty"` // Timestamps
	PointerType string    `json:"pointerType,omitempty"`
}

