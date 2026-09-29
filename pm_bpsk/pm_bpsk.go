// Package pm_bpsk modulates bits using binary phase-shift keying (BPSK).
package pm_bpsk

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

// Processor retains the underlying carrier phase across bits and chunks.
// Its zero value is ready to use. Use a fresh processor per file, not concurrently.
type Processor struct {
	phase     float64
	carrierHz *float64
}

// New selects a carrier without restricting spacing or bandwidth.
// Zero, negative, and above-Nyquist values are allowed for experimentation.
func New(carrierHz float64) (*Processor, error) {
	if math.IsNaN(carrierHz) || math.IsInf(carrierHz, 0) {
		return nil, fmt.Errorf("carrier frequency must be finite")
	}
	return &Processor{carrierHz: &carrierHz}, nil
}

func (*Processor) SampleRate() int    { return sampleRate }
func (*Processor) SamplesPerBit() int { return samplesPerBit }

// Modulate maps 0 to 0 degrees and 1 to 180 degrees of phase offset.
// Each bit lasts 480 samples. It returns a new slice without changing the input.
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
	phaseStep := 2 * math.Pi * (carrier / sampleRate)
	for i, bit := range bits {
		// Negating a sine is equivalent to shifting its phase by 180 degrees.
		polarity := 1 - 2*float64(bit)
		for sample := 0; sample < samplesPerBit; sample++ {
			output[i*samplesPerBit+sample] = polarity * math.Sin(p.phase)
			// Keep the carrier running; only the bit's phase offset changes.
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
