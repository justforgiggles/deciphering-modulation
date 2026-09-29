// Package am modulates bits using on/off keying, a binary form of AM.
package am

import (
	"fmt"
	"math"
)

const (
	CarrierHz     = 1070
	BitRate       = 100
	sampleRate    = 48000
	samplesPerBit = sampleRate / BitRate
)

// Processor retains the carrier phase across chunks. Its zero value is ready
// to use. Use a fresh processor for each file; do not use it concurrently.
type Processor struct {
	phase float64
}

func (*Processor) SampleRate() int    { return sampleRate }
func (*Processor) SamplesPerBit() int { return samplesPerBit }

// Modulate converts bits (each byte must be 0 or 1) to normalized audio samples.
// Each bit lasts 480 samples: zero is silence, one is a 1,070 Hz sine wave.
// The output is newly allocated and the input is unchanged. Invalid input leaves
// the carrier phase unchanged.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	output := make([]float64, len(bits)*samplesPerBit)
	phaseStep := 2 * math.Pi * CarrierHz / sampleRate
	for i, bit := range bits {
		for sample := 0; sample < samplesPerBit; sample++ {
			// The bit switches the carrier's amplitude between silence and full volume.
			output[i*samplesPerBit+sample] = float64(bit) * math.Sin(p.phase)
			// The carrier advances even during silence, and never restarts at a chunk.
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
