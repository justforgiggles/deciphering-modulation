// Package qam modulates groups of four bits using square 16-QAM.
package qam

import (
	"fmt"
	"math"
)

const (
	CarrierHz        = 1070
	BitRate          = 100
	sampleRate       = 48000
	samplesPerBit    = sampleRate / BitRate
	samplesPerSymbol = 4 * samplesPerBit
)

// Processor retains carrier phase across symbols and chunks. Its zero value is
// ready to use. Use a fresh processor per file, not concurrently.
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

// Modulate maps four bits to I/Q amplitudes: the first pair selects I, the last Q.
// Each axis uses Gray mapping: 00 -> -3, 01 -> -1, 11 -> +1, 10 -> +3.
// Input must contain complete four-bit symbols. Invalid input leaves state
// unchanged. The returned slice is new and the input is never modified.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if len(bits)%4 != 0 {
		return nil, fmt.Errorf("16-QAM requires complete four-bit symbols: got %d bits", len(bits))
	}
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	// Indexed by the binary pair 00, 01, 10, 11.
	levels := [4]float64{-3, -1, 3, 1}
	carrier := float64(CarrierHz)
	if p.carrierHz != nil {
		carrier = *p.carrierHz
	}
	output := make([]float64, len(bits)*samplesPerBit)
	phaseStep := 2 * math.Pi * (carrier / sampleRate)
	for i := 0; i < len(bits); i += 4 {
		inPhase := levels[bits[i]*2+bits[i+1]]
		quadrature := levels[bits[i+2]*2+bits[i+3]]
		for sample := 0; sample < samplesPerSymbol; sample++ {
			// Orthogonal carriers combine amplitude and phase. The largest symbol
			// has radius 3*sqrt(2); normalize once to keep every sample in [-1, 1].
			value := (inPhase*math.Cos(p.phase) - quadrature*math.Sin(p.phase)) / (3 * math.Sqrt2)
			// Floating-point rounding at a full-scale peak can exceed 1 by an ulp.
			output[i*samplesPerBit+sample] = max(-1, min(1, value))
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
