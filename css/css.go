// Package css modulates bits using binary chirp spread spectrum.
package css

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

// Modulate maps 0 to a linear up-chirp from center-250 Hz to center+250 Hz,
// and 1 to the reverse down-chirp, with a constant unit envelope.
// Each bit lasts SampleRate/BitRate samples. Phase stays continuous when the
// sweep restarts at each bit. Output is newly allocated and input is unchanged.
// Invalid input returns an error without advancing phase.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if p.samplesPerBit == 0 {
		return nil, fmt.Errorf("processor must be initialized with New")
	}
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	if len(bits) > int(^uint(0)>>1)/p.samplesPerBit {
		return nil, fmt.Errorf("input produces too many samples")
	}
	const bandwidthHz = 500
	samplesPerBit := p.samplesPerBit
	output := make([]float64, len(bits)*samplesPerBit)
	rate := float64(p.config.SampleRate)
	// Reduce the carrier first so extreme finite values retain the sweep offset.
	carrier := math.Mod(p.config.CarrierHz, rate)
	for i, bit := range bits {
		direction := 1 - 2*float64(bit)
		for sample := 0; sample < samplesPerBit; sample++ {
			output[i*samplesPerBit+sample] = math.Sin(p.phase)
			// Midpoint frequency integrates the linear sweep exactly over this interval.
			frequency := carrier + direction*bandwidthHz*((float64(sample)+0.5)/float64(samplesPerBit)-0.5)
			phaseStep := 2 * math.Pi * (math.Mod(frequency, rate) / rate)
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
