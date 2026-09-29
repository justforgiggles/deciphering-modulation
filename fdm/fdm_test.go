package fdm

import (
	"math"
	"slices"
	"testing"
)

func TestMix(t *testing.T) {
	a, b := []float64{1, -1, .5}, []float64{1, 1}
	for _, inputs := range [][2][]float64{{a, b}, {b, a}} {
		got, err := Mix(inputs[0], inputs[1])
		if err != nil || !slices.Equal(got, []float64{1, 0, .25}) {
			t.Fatalf("mix: %v, %v", got, err)
		}
		got[0] = 0
		if a[0] != 1 || b[0] != 1 {
			t.Fatal("modified input")
		}
	}
	got, err := Mix(nil, nil)
	if err != nil || len(got) != 0 {
		t.Fatal("empty mix")
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1.01, -1.01} {
		for _, inputs := range [][2][]float64{{{bad}, nil}, {nil, {bad}}} {
			if _, err := Mix(inputs[0], inputs[1]); err == nil {
				t.Fatal("accepted invalid sample")
			}
		}
	}
}
