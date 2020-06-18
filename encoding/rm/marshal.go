package rm

import (
	"bytes"
	"encoding/binary"
)

// MarshalBinary implements encoding.MarshalBinary for
// transforming a Rm page into bytes
// TODO
func (rm *Rm) MarshalBinary() (data []byte, err error) {
	w := new(writer)

	w.writeHeader()

	nbLayers := len(rm.Layers)
	w.writeNumber(nbLayers)

	for _, layer := range rm.Layers {
		nbLines := len(layer.Lines)
		w.writeNumber(nbLines)

		for _, line := range layer.Lines {
			w.writeLine(line)
		}
	}
	data = w.Bytes()

	return
}

type writer struct {
	b bytes.Buffer
}

func (r *writer) Bytes() []byte {
	return r.b.Bytes()
}

func (r *writer) writeHeader() error {
	r.b.Write([]byte(HeaderV5))
	return nil
}

func (r *writer) writeNumber(n int) error {
	binary.Write(&r.b, binary.LittleEndian, uint32(n))
	return nil
}

func (r *writer) writeFloat32(n float32) error {
	binary.Write(&r.b, binary.LittleEndian, n)
	return nil
}

func (r *writer) writeLine(line Line) error {

	r.writeNumber(int(line.BrushType))
	r.writeFloat32(float32(line.BrushColor))
	r.writeFloat32(float32(line.Padding))
	r.writeFloat32(0.0)
	r.writeFloat32(float32(line.BrushSize))

	nbPoints := len(line.Points)
	r.writeNumber(nbPoints)

	for _, point := range line.Points {
		r.writePoint(point)
	}

	return nil
}

func (r *writer) writePoint(point Point) error {
	r.writeFloat32(point.X)
	r.writeFloat32(point.Y)
	r.writeFloat32(point.Speed)
	r.writeFloat32(point.Direction)
	r.writeFloat32(point.Width)
	r.writeFloat32(point.Pressure)

	return nil
}
