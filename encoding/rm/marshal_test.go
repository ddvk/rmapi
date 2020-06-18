package rm

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

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
						Points:     points,
					},
					Line{
						BrushSize:  Large,
						BrushColor: Black,
						BrushType:  FinelinerV5,
						Points: []Point{
							Point{
								X:         100,
								Y:         100,
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
