// Package qam modulates groups of four bits using square 16-QAM.
package qam

import (
	"fmt"
	"math"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

// Processor retains carrier phase across symbols and chunks. Construct it with
// New and use a fresh processor for each file; do not use it concurrently.
type Processor struct {
	config        modulation.Config
	samplesPerBit int
	phase         float64
}

// New configures carrier and timing without restricting spacing or bandwidth.
// Zero, negative, and above-Nyquist carriers are allowed for experimentation.
func New(config modulation.Config) (*Processor, error) {
	if math.IsNaN(config.CarrierHz) || math.IsInf(config.CarrierHz, 0) {
		return nil, fmt.Errorf("carrier frequency must be finite")
	}
	if config.SampleRate <= 0 || config.BitRate <= 0 || config.SampleRate%config.BitRate != 0 {
		return nil, fmt.Errorf("sample rate and bit rate must be positive, with sample rate divisible by bit rate")
	}
	return &Processor{config: config, samplesPerBit: config.SampleRate / config.BitRate}, nil
}

func (p *Processor) SampleRate() int    { return p.config.SampleRate }
func (p *Processor) SamplesPerBit() int { return p.samplesPerBit }

// Modulate maps four bits to I/Q amplitudes: the first pair selects I, the last Q.
// Each axis uses Gray mapping: 00 -> -3, 01 -> -1, 11 -> +1, 10 -> +3.
// Input must contain complete four-bit symbols. Invalid input leaves state
// unchanged. The returned slice is new and the input is never modified.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if p.samplesPerBit == 0 {
		return nil, fmt.Errorf("processor must be initialized with New")
	}
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
	// Reject sample counts that would overflow before allocating output.
	if len(bits) > int(^uint(0)>>1)/p.samplesPerBit {
		return nil, fmt.Errorf("input produces too many samples")
	}
	samplesPerBit := p.samplesPerBit
	const bitsPerSymbol = 4
	samplesPerSymbol := bitsPerSymbol * samplesPerBit
	output := make([]float64, len(bits)*samplesPerBit)
	// Reduce aliased frequencies before scaling to avoid floating-point overflow.
	phaseStep := 2 * math.Pi * (math.Mod(p.config.CarrierHz, float64(p.config.SampleRate)) / float64(p.config.SampleRate))
	for i := 0; i < len(bits); i += bitsPerSymbol {
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
