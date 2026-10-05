// Package css_16 modulates four-bit symbols using 16 cyclically shifted up-chirps.
// The fixed 500 Hz sweep is an educational CSS waveform, not a LoRa format.
package css_16

import (
	"fmt"
	"math"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

// Processor retains phase across symbols and chunks. Construct it with New
// and use a fresh processor for each file; do not use it concurrently.
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

// Modulate maps MSB-first four-bit symbols k to up-chirps shifted by k/16
// of a sweep. Each symbol lasts 4/BitRate seconds, sweeping within center±250 Hz
// and wrapping from the upper edge to the lower edge with continuous phase.
// Input must contain complete four-bit symbols. Invalid input leaves state
// unchanged. The returned slice is new and input is never modified.
func (p *Processor) Modulate(bits []byte) ([]float64, error) {
	if p.samplesPerBit == 0 {
		return nil, fmt.Errorf("processor must be initialized with New")
	}
	if len(bits)%4 != 0 {
		return nil, fmt.Errorf("16-symbol CSS requires complete four-bit symbols: got %d bits", len(bits))
	}
	for i, bit := range bits {
		if bit > 1 {
			return nil, fmt.Errorf("bit %d is %d: expected 0 or 1", i, bit)
		}
	}
	// Check before multiplying samples per bit by four, including empty input.
	if len(bits) > int(^uint(0)>>1)/p.samplesPerBit {
		return nil, fmt.Errorf("input produces too many samples")
	}
	output := make([]float64, len(bits)*p.samplesPerBit)
	if len(bits) == 0 {
		return output, nil
	}
	const bandwidthHz = 500
	samplesPerSymbol := 4 * p.samplesPerBit
	rate := float64(p.config.SampleRate)
	carrier := math.Mod(p.config.CarrierHz, rate)
	for i := 0; i < len(bits); i += 4 {
		symbol := bits[i]<<3 | bits[i+1]<<2 | bits[i+2]<<1 | bits[i+3]
		shift := float64(symbol) / 16
		wrapSample := (1 - shift) * float64(samplesPerSymbol)
		for sample := 0; sample < samplesPerSymbol; sample++ {
			output[i*p.samplesPerBit+sample] = math.Sin(p.phase)
			// Integrate the unwrapped ramp at the interval midpoint, then subtract
			// the band jump weighted by the fraction of this interval after wrap.
			// This also handles wraps that lie between two sample instants.
			afterWrap := max(0, min(1, float64(sample)+1-wrapSample))
			frequency := carrier + bandwidthHz*((float64(sample)+0.5)/float64(samplesPerSymbol)+shift-0.5-afterWrap)
			phaseStep := 2 * math.Pi * (math.Mod(frequency, rate) / rate)
			p.phase = math.Mod(p.phase+phaseStep, 2*math.Pi)
		}
	}
	return output, nil
}
