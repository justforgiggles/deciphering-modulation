// Package demodulation demonstrates an offline receiver for this repository's
// binary up/down-chirp protocol. See README.md for the complete signal path.
package demodulation

import (
	"errors"
	"math"
)

// IQ stores two real arrays rather than hiding quadrature mixing in complex math.
// I is the cosine branch; Q is the negative-sine branch.
type IQ struct {
	I, Q       []float64
	SampleRate float64
}

type Config struct{ CarrierHz, BandwidthHz, BitRate float64 }

func DefaultConfig() Config { return Config{6000, 500, 100} }

// Validate bounds the demonstrated operating range and keeps all searched
// chirps below Nyquist. It must precede the numeric processing stages.
func (c Config) Validate() error {
	for _, v := range []float64{c.CarrierHz, c.BandwidthHz, c.BitRate} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return errors.New("carrier, bandwidth and bit rate must be finite and positive")
		}
	}
	if c.BitRate < 20 || c.BitRate > 500 || c.BandwidthHz < 100 || c.BandwidthHz > 2000 {
		return errors.New("supported range: 20–500 bits/s and 100–2000 Hz bandwidth")
	}
	margin := c.BandwidthHz/2 + 200
	if c.CarrierHz <= margin || c.CarrierHz+margin >= SampleRate/2 {
		return errors.New("carrier and search band must fit below Nyquist")
	}
	return nil
}

// RemoveDCOffset subtracts the constant (zero-frequency) part of the recording.
// It does not normalize volume or gate quiet samples.
func RemoveDCOffset(audio Audio) Audio {
	var mean float64
	for _, sample := range audio.Samples {
		mean += sample
	}
	if len(audio.Samples) > 0 {
		mean /= float64(len(audio.Samples))
	}
	out := Audio{make([]float64, len(audio.Samples)), audio.SampleRate}
	for i, sample := range audio.Samples {
		out.Samples[i] = sample - mean
	}
	return out
}

// GenerateQuadratureCarrier makes the actual local-oscillator sample arrays.
func GenerateQuadratureCarrier(count int, sampleRate, carrierHz float64) IQ {
	carrier := IQ{make([]float64, count), make([]float64, count), sampleRate}
	for i := range carrier.I {
		phase := 2 * math.Pi * carrierHz * float64(i) / sampleRate
		carrier.I[i] = math.Cos(phase)
		carrier.Q[i] = -math.Sin(phase)
	}
	return carrier
}

// MixToBaseband multiplies sample-by-sample. A real input produces both the
// wanted difference-frequency component and an unwanted sum-frequency image.
// audio and carrier must have matching lengths and rates.
func MixToBaseband(audio Audio, carrier IQ) IQ {
	out := IQ{make([]float64, len(audio.Samples)), make([]float64, len(audio.Samples)), audio.SampleRate}
	for i, sample := range audio.Samples {
		out.I[i] = sample * carrier.I[i]
		out.Q[i] = sample * carrier.Q[i]
	}
	return out
}

// DownsampleFactor retains at least 12 samples per symbol and room for the
// sweep plus the ±200 Hz search. At the defaults, 48000/16 = 3000 samples/s.
func DownsampleFactor(sampleRate float64, c Config) int {
	target := math.Max(3000, math.Max(12*c.BitRate, 4*c.BandwidthHz))
	return max(1, int(sampleRate/target))
}

// DesignLowPassFilter creates a symmetric Hamming-windowed sinc. The sinc is
// the ideal low-pass impulse response; the window makes a finite approximation.
// Scaling the taps to sum to one preserves the amplitude of a constant signal.
func DesignLowPassFilter(sampleRate float64, factor int) []float64 {
	half := 16 * factor
	newSampleRate := sampleRate / float64(factor)
	cutoffHz := .4 * newSampleRate
	cutoff := cutoffHz / sampleRate // Convert Hz to cycles per original sample.
	taps := make([]float64, 2*half+1)
	var sum float64
	for i := range taps {
		n := i - half
		sinc := 2 * cutoff
		if n != 0 {
			sinc = math.Sin(2*math.Pi*cutoff*float64(n)) / (math.Pi * float64(n))
		}
		window := .54 + .46*math.Cos(math.Pi*float64(n)/float64(half))
		taps[i] = sinc * window
		sum += taps[i]
	}
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}

// ApplyFIRFilter explicitly convolves both branches. This offline, centered
// filter pads with zeros at the ends and compensates its half-length delay:
// sample zero still refers to time zero in the original recording.
func ApplyFIRFilter(input IQ, taps []float64) IQ {
	out := IQ{make([]float64, len(input.I)), make([]float64, len(input.I)), input.SampleRate}
	half := len(taps) / 2
	for i := range out.I {
		for tap := max(0, half-i); tap < min(len(taps), len(input.I)-i+half); tap++ {
			index := i + tap - half
			out.I[i] += input.I[index] * taps[tap]
			out.Q[i] += input.Q[index] * taps[tap]
		}
	}
	return out
}

// Downsample keeps every Nth sample AFTER filtering to prevent aliasing.
func Downsample(input IQ, factor int) IQ {
	n := (len(input.I) + factor - 1) / factor
	out := IQ{make([]float64, n), make([]float64, n), input.SampleRate / float64(factor)}
	for i := range out.I {
		out.I[i] = input.I[i*factor]
		out.Q[i] = input.Q[i*factor]
	}
	return out
}

// GenerateReferenceChirp integrates midpoint frequencies to build a sample
// array. 0 sweeps upward, 1 downward; offsetHz represents residual carrier error.
func GenerateReferenceChirp(period, rate, bandwidth, offsetHz float64, bit byte) IQ {
	n := int(math.Round(period))
	ref := IQ{make([]float64, n), make([]float64, n), rate}
	phase := 0.0
	direction := 1 - 2*float64(bit)
	for i := range ref.I {
		ref.I[i] = math.Cos(phase)
		ref.Q[i] = math.Sin(phase)
		frequency := offsetHz + direction*bandwidth*((float64(i)+.5)/period-.5)
		phase += 2 * math.Pi * frequency / rate
	}
	return ref
}

// Dechirp multiplies received I+jQ by the conjugate of the reference.
// Matching sweeps cancel, leaving a nearly constant phasor; mismatched sweeps
// keep rotating and mostly cancel when their samples are summed.
func Dechirp(received, reference IQ) IQ {
	out := IQ{make([]float64, len(reference.I)), make([]float64, len(reference.I)), received.SampleRate}
	for i := range out.I {
		out.I[i] = received.I[i]*reference.I[i] + received.Q[i]*reference.Q[i]
		out.Q[i] = received.Q[i]*reference.I[i] - received.I[i]*reference.Q[i]
	}
	return out
}

// MeasureCoherentEnergy is squared magnitude of the summed dechirped samples.
// Summing I and Q separately preserves energy for any unknown input phase.
func MeasureCoherentEnergy(dechirped IQ) float64 {
	var iSum, qSum float64
	for i := range dechirped.I {
		iSum += dechirped.I[i]
		qSum += dechirped.Q[i]
	}
	return iSum*iSum + qSum*qSum
}

// InterpolateSymbol creates a symbol window at a fractional sample position.
// Linear interpolation avoids rounding the timing estimate at every bit.
func InterpolateSymbol(input IQ, start float64, count int) (IQ, bool) {
	index := int(math.Floor(start))
	fraction := start - float64(index)
	if index < 0 || count <= 0 || index+count > len(input.I) || (fraction != 0 && index+count >= len(input.I)) {
		return IQ{}, false
	}
	out := IQ{make([]float64, count), make([]float64, count), input.SampleRate}
	for i := range out.I {
		out.I[i] = input.I[index+i]
		out.Q[i] = input.Q[index+i]
		if fraction != 0 {
			out.I[i] += fraction * (input.I[index+i+1] - input.I[index+i])
			out.Q[i] += fraction * (input.Q[index+i+1] - input.Q[index+i])
		}
	}
	return out, true
}
