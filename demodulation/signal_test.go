package demodulation

import (
	"math"
	"testing"
)

func TestDCAndMixer(t *testing.T) {
	input := Audio{[]float64{2, 3, 4}, 48000}
	centered := RemoveDCOffset(input)
	if centered.Samples[0] != -1 || centered.Samples[1] != 0 || centered.Samples[2] != 1 || input.Samples[0] != 2 {
		t.Fatal("DC subtraction changed the signal shape or input")
	}
	carrier := GenerateQuadratureCarrier(8, 48000, 6000)
	for i := range carrier.I {
		if math.Abs(carrier.I[i]*carrier.I[i]+carrier.Q[i]*carrier.Q[i]-1) > 1e-12 {
			t.Fatal("oscillator is not unit magnitude")
		}
	}
	mixed := MixToBaseband(Audio{carrier.I, 48000}, carrier)
	var iMean, qMean float64
	for i := range mixed.I {
		iMean += mixed.I[i] / 8
		qMean += mixed.Q[i] / 8
	}
	if math.Abs(iMean-.5) > 1e-12 || math.Abs(qMean) > 1e-12 {
		t.Fatal("mixing did not move the carrier to DC")
	}
}

func TestFIRAndDownsampling(t *testing.T) {
	taps := DesignLowPassFilter(48000, 16)
	response := func(frequency float64) float64 {
		var real, imag float64
		for i, tap := range taps {
			phase := 2 * math.Pi * frequency * float64(i) / 48000
			real += tap * math.Cos(phase)
			imag += tap * math.Sin(phase)
		}
		return math.Hypot(real, imag)
	}
	if math.Abs(response(0)-1) > 1e-12 || response(450) < .98 || response(3000) > .01 || response(12000) > .01 {
		t.Fatal("filter passband or stopband incorrect")
	}
	x := IQ{make([]float64, 2048), make([]float64, 2048), 48000}
	x.I[1024] = 1
	x.Q[1024] = .5
	y := ApplyFIRFilter(x, taps)
	peak := 0
	for i := range y.I {
		if math.Abs(y.I[i]) > math.Abs(y.I[peak]) {
			peak = i
		}
	}
	if peak != 1024 {
		t.Fatal("filter shifted timestamps")
	}
	z := Downsample(y, 16)
	if z.SampleRate != 3000 || len(z.I) != 128 || z.I[64] != y.I[1024] || z.Q[64] != y.Q[1024] {
		t.Fatal("incorrect decimation")
	}
}

func TestDechirpingAndInterpolation(t *testing.T) {
	for _, bit := range []byte{0, 1} {
		reference := GenerateReferenceChirp(30, 3000, 500, 35, bit)
		for _, phase := range []float64{0, .37, 2.1} {
			received := IQ{make([]float64, 30), make([]float64, 30), 3000}
			for i := range received.I {
				received.I[i] = reference.I[i]*math.Cos(phase) - reference.Q[i]*math.Sin(phase)
				received.Q[i] = reference.I[i]*math.Sin(phase) + reference.Q[i]*math.Cos(phase)
			}
			if math.Abs(MeasureCoherentEnergy(Dechirp(received, reference))-900) > 1e-8 {
				t.Fatal("phase changed dechirped energy")
			}
			other := GenerateReferenceChirp(30, 3000, 500, 35, 1-bit)
			if MeasureCoherentEnergy(Dechirp(received, other)) > 200 {
				t.Fatal("opposite chirp was not rejected")
			}
		}
	}
	input := IQ{[]float64{0, 2, 4}, []float64{0, 4, 8}, 3000}
	window, ok := InterpolateSymbol(input, .5, 2)
	if !ok || window.I[0] != 1 || window.I[1] != 3 || window.Q[1] != 6 {
		t.Fatal("fractional interpolation failed")
	}
	if _, ok := InterpolateSymbol(input, 1.5, 2); ok {
		t.Fatal("accepted out-of-range interpolation")
	}
}

func TestConfigValidation(t *testing.T) {
	for _, c := range []Config{{math.NaN(), 500, 100}, {6000, math.Inf(1), 100}, {6000, 500, 0}, {6000, 500, 501}, {200, 500, 100}, {23900, 500, 100}} {
		if c.Validate() == nil {
			t.Fatalf("accepted invalid config %+v", c)
		}
	}
}
