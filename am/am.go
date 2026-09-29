// Package am modulates bits using on/off keying, a binary form of AM.
package am

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

// Modulate converts bits (each byte must be 0 or 1) to normalized audio samples.
// Each bit lasts SampleRate/BitRate samples: zero is silence, one is a sine wave at the selected carrier.
// The output is newly allocated and the input is unchanged. Invalid input leaves
// the carrier phase unchanged.
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
		for sample := 0; sample < samplesPerSymbol; sample++ {
			// The bit switches the carrier's amplitude between silence and full volume.
			output[i*samplesPerBit+sample] = float64(bit) * math.Sin(p.phase)
			// The carrier advances even during silence, and never restarts at a chunk.
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
