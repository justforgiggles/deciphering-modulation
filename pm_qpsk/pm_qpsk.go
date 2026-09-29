// Package pm_qpsk modulates pairs of bits using quadrature phase-shift keying.
package pm_qpsk

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

// Modulate maps bit pairs to Gray-coded phase offsets:
// 00 -> 45 degrees, 01 -> 135 degrees, 11 -> 225 degrees, 10 -> 315 degrees.
// Each symbol lasts 2*SampleRate/BitRate samples. Input must contain complete pairs; invalid
// input leaves state unchanged. Output is newly allocated and input is unchanged.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if p.samplesPerBit == 0 {
		return nil, fmt.Errorf("processor must be initialized with New")
	}
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
	// Reject sample counts that would overflow before allocating output.
	if len(bits) > int(^uint(0)>>1)/p.samplesPerBit {
		return nil, fmt.Errorf("input produces too many samples")
	}
	samplesPerBit := p.samplesPerBit
	const bitsPerSymbol = 2
	samplesPerSymbol := bitsPerSymbol * samplesPerBit
	output := make([]float64, len(bits)*samplesPerBit)
	// Reduce aliased frequencies before scaling to avoid floating-point overflow.
	phaseStep := 2 * math.Pi * (math.Mod(p.config.CarrierHz, float64(p.config.SampleRate)) / float64(p.config.SampleRate))
	for i := 0; i < len(bits); i += bitsPerSymbol {
		pair := bits[i]*2 + bits[i+1]
		for sample := 0; sample < samplesPerSymbol; sample++ {
			output[i*samplesPerBit+sample] = math.Sin(p.phase + phaseOffsets[pair])
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
