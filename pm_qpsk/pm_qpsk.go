// Package pm_qpsk modulates pairs of bits using quadrature phase-shift keying.
package pm_qpsk

import (
	"fmt"
	"math"
)

const (
	CarrierHz        = 1070
	BitRate          = 100
	sampleRate       = 48000
	samplesPerBit    = sampleRate / BitRate
	samplesPerSymbol = 2 * samplesPerBit
)

// Processor retains the underlying carrier phase across symbols and chunks.
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

// Modulate maps bit pairs to Gray-coded phase offsets:
// 00 -> 45 degrees, 01 -> 135 degrees, 11 -> 225 degrees, 10 -> 315 degrees.
// Each symbol lasts 960 samples. Input must contain complete pairs; invalid
// input leaves state unchanged. Output is newly allocated and input is unchanged.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if len(bits)%2 != 0 {
		return nil, fmt.Errorf("QPSK requires complete bit pairs: got %d bits", len(bits))
	}
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	// Indexed by the binary value of the pair: 00, 01, 10, 11.
	phaseOffsets := [4]float64{math.Pi / 4, 3 * math.Pi / 4, 7 * math.Pi / 4, 5 * math.Pi / 4}
	carrier := float64(CarrierHz)
	if p.carrierHz != nil {
		carrier = *p.carrierHz
	}
	output := make([]float64, len(bits)*samplesPerBit)
	phaseStep := 2 * math.Pi * (carrier / sampleRate)
	for i := 0; i < len(bits); i += 2 {
		pair := bits[i]*2 + bits[i+1]
		for sample := 0; sample < samplesPerSymbol; sample++ {
			output[i*samplesPerBit+sample] = math.Sin(p.phase + phaseOffsets[pair])
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
