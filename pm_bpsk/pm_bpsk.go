// Package pm_bpsk modulates bits using binary phase-shift keying (BPSK).
package pm_bpsk

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

// Modulate maps 0 to 0 degrees and 1 to 180 degrees of phase offset.
// Each bit lasts SampleRate/BitRate samples. It returns a new slice without changing the input.
// Invalid bits return an error without advancing the carrier phase.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if p.samplesPerBit == 0 {
		return nil, fmt.Errorf("processor must be initialized with New")
	}
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	// Reject sample counts that would overflow before allocating output.
	if len(bits) > int(^uint(0)>>1)/p.samplesPerBit {
		return nil, fmt.Errorf("input produces too many samples")
	}
	samplesPerBit := p.samplesPerBit
	const bitsPerSymbol = 1
	samplesPerSymbol := bitsPerSymbol * samplesPerBit
	output := make([]float64, len(bits)*samplesPerBit)
	// Reduce aliased frequencies before scaling to avoid floating-point overflow.
	phaseStep := 2 * math.Pi * (math.Mod(p.config.CarrierHz, float64(p.config.SampleRate)) / float64(p.config.SampleRate))
	for i := 0; i < len(bits); i += bitsPerSymbol {
		bit := bits[i]
		// Negating a sine is equivalent to shifting its phase by 180 degrees.
		polarity := 1 - 2*float64(bit)
		for sample := 0; sample < samplesPerSymbol; sample++ {
			output[i*samplesPerBit+sample] = polarity * math.Sin(p.phase)
			// Keep the carrier running; only the bit's phase offset changes.
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
