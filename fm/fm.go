// Package fm modulates bits using continuous-phase binary frequency-shift keying.
package fm

import (
	"fmt"
	"math"
)

const (
	CarrierHz     = 1070
	DeviationHz   = 250
	BitRate       = 100
	sampleRate    = 48000
	samplesPerBit = sampleRate / BitRate
)

// Processor retains carrier phase across bits and chunks. Its zero value is
// ready to use. Use a fresh processor for each file; do not use it concurrently.
type Processor struct {
	phase     float64
	carrierHz *float64
}

// New selects a carrier (or FM center) without restricting spacing or bandwidth.
// Zero, negative, and above-Nyquist values are allowed for experimentation.
func New(carrierHz float64) (*Processor, error) {
	if math.IsNaN(carrierHz) || math.IsInf(carrierHz, 0) {
		return nil, fmt.Errorf("carrier frequency must be finite")
	}
	return &Processor{carrierHz: &carrierHz}, nil
}

func (*Processor) SampleRate() int    { return sampleRate }
func (*Processor) SamplesPerBit() int { return samplesPerBit }

// Modulate maps 0 to center+DeviationHz and 1 to center-DeviationHz, with a constant unit envelope.
// Each bit lasts 480 samples. The output is newly allocated and input is unchanged.
// Invalid bits return an error without advancing the carrier phase.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	carrier := float64(CarrierHz)
	if p.carrierHz != nil {
		carrier = *p.carrierHz
	}
	output := make([]float64, len(bits)*samplesPerBit)
	for i, bit := range bits {
		frequency := carrier + (1-2*float64(bit))*DeviationHz
		phaseStep := 2 * math.Pi * (frequency / sampleRate)
		for sample := 0; sample < samplesPerBit; sample++ {
			// Change frequency, keeping amplitude and phase continuous.
			output[i*samplesPerBit+sample] = math.Sin(p.phase)
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
