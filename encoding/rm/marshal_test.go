package rm

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"testing"
)

func drawCircle(x, y, r float64) []Point {
	theta := 0.0
	dtheta := math.Pi / 100
	points := make([]Point, 0)
	for theta < 2*math.Pi {
		cx := x + r*math.Cos(theta)
		cy := y + r*math.Sin(theta)

		p := Point{
			X:         float32(cx),
			Y:         float32(cy),
			Speed:     2.,
			Direction: 3.,
			Width:     2.0,
			Pressure:  .3,
		}
		points = append(points, p)
		theta += dtheta
	}
	return points
}

func newPoint(x, y float32) Point {
	p := Point{
		X:         x,
		Y:         y,
		Speed:     2.,
		Direction: 3.,
		Width:     2.0,
		Pressure:  .3,
	}
	return p
}
func drawRectange(x, y, w, h float32) []Point {
	points := make([]Point, 0)
	p := newPoint(x, y)
	points = append(points, p)
	p = newPoint(x+w, y)
	points = append(points, p)
	p = newPoint(x+w, y+h)
	points = append(points, p)
	p = newPoint(x, y+h)
	points = append(points, p)
	p = newPoint(x, y)
	points = append(points, p)
	return points
}
func testMarshalBinary(t *testing.T, fn string) {

	points := make([]Point, 0)
	for i := 0; i < 200; i++ {
		c := float32(i)

		p := Point{
			X:         100,
			Y:         c,
			Speed:     2.,
			Direction: 3.,
			Width:     2.0,
			Pressure:  .3,
		}
		points = append(points, p)
	}

	rm := Rm{
		Layers: []Layer{
			Layer{
				Lines: []Line{
					Line{
						BrushSize:  Medium,
						BrushColor: Black,
						BrushType:  FinelinerV5,
						Points:     drawCircle(500, 500, 100),
					},
					Line{
						BrushSize:  Medium,
						BrushColor: Black,
						BrushType:  FinelinerV5,
						Points:     drawRectange(400, 50, 200, 200),
					},
					Line{
						BrushSize:  Large,
						BrushColor: Black,
						BrushType:  FinelinerV5,
						Points: []Point{
							Point{
								X:         100,
								Y:         400,
								Speed:     2.,
								Direction: 1.,
								Width:     3.0,
								Pressure:  .3,
							},
							Point{
								X:         1000,
								Y:         1000,
								Speed:     2.,
								Direction: 1.,
								Width:     3.0,
								Pressure:  .3,
							},
						},
					},
				},
			},
		},
	}

	data, err := rm.MarshalBinary()
	if err != nil {
		t.Error(err)
	}

	err = ioutil.WriteFile(fn, data, os.ModePerm)
	if err != nil {
		t.Errorf("can't write %s file", fn)
	}
	t.Log(rm)

	fmt.Println("unmarshaling complete")
}

func TestMarshalBinary(t *testing.T) {
	testMarshalBinary(t, "/tmp/rmapi_test.rm")
}
