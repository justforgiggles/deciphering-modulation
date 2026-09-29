// Package fdm combines independently modulated audio channels.
package fdm

import (
	"fmt"
	"math"
)

// Mix returns (first+second)/2, treating missing samples as silence.
// Inputs are unchanged. Gain stays fixed when one channel ends. No filtering
// or frequency separation is applied; overlapping carriers mix as supplied.
func Mix(first, second []float64) ([]float64, error) {
	output := make([]float64, max(len(first), len(second)))
	for channel, samples := range [][]float64{first, second} {
		for i, sample := range samples {
			if math.IsNaN(sample) || math.IsInf(sample, 0) || sample < -1 || sample > 1 {
				return nil, fmt.Errorf("channel %d sample %d must be finite and in [-1, 1]", channel+1, i)
			}
			output[i] += sample / 2
		}
	}
	return output, nil
}
