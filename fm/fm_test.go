package fm

import (
	"math"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

const (
	CarrierHz   = 1070
	BitRate     = 100
	sampleRate  = 48000
	DeviationHz = 250
)

func newTestProcessor(t *testing.T) *Processor {
	t.Helper()
	p, err := New(modulation.Config{CarrierHz: CarrierHz, SampleRate: sampleRate, BitRate: BitRate})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var _ modulation.Processor = (*Processor)(nil)

func TestModulate(t *testing.T) {
	var uninitialized Processor
	if uninitialized.SampleRate() != 0 || uninitialized.SamplesPerBit() != 0 {
		t.Fatal("uninitialized processor reported timing")
	}
	for _, bits := range [][]byte{nil, {0, 0, 0, 0}} {
		if _, err := uninitialized.Modulate(bits); err == nil {
			t.Fatal("uninitialized processor accepted input")
		}
	}
	const spb = sampleRate / BitRate
	bits := []byte{0, 1, 1, 0, 1, 0, 0}
	original := slices.Clone(bits)
	p := newTestProcessor(t)
	got, err := p.Modulate(bits)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleRate() != sampleRate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
		t.Fatal("incorrect timing or sample count")
	}
	// Compare with integrated frequency, independently of the wrapped accumulator.
	cycles := 0.0
	for i, bit := range bits {
		frequency := float64(CarrierHz + DeviationHz)
		if bit == 1 {
			frequency = CarrierHz - DeviationHz
		}
		energy, wantEnergy := 0.0, 0.0
		for j, sample := range got[i*spb : (i+1)*spb] {
			want := math.Sin(2 * math.Pi * (cycles + float64(j)*frequency/sampleRate))
			if math.IsNaN(sample) || math.Abs(sample) > 1 || math.Abs(sample-want) > 1e-11 {
				t.Fatalf("bit %d sample %d: got %g, want %g", i, j, sample, want)
			}
			energy += sample * sample
			wantEnergy += want * want
		}
		if math.Abs(energy/spb-wantEnergy/spb) > 1e-11 {
			t.Fatal("carrier power differs from the expected sine")
		}
		cycles += frequency / BitRate
	}
	// The next symbol must retain the phase accumulated during the first tone.
	if math.Abs(got[spb]-math.Sin(2*math.Pi*(CarrierHz+DeviationHz)/BitRate)) > 1e-11 {
		t.Fatal("phase reset at a frequency transition")
	}
	if !slices.Equal(bits, original) {
		t.Fatal("input changed")
	}
	chunked := newTestProcessor(t)
	var joined []float64
	for _, chunk := range [][]byte{bits[:1], nil, bits[1:4], bits[4:]} {
		audio, err := chunked.Modulate(chunk)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) {
		t.Fatal("chunk boundaries changed output")
	}
	phase := p.phase
	for _, invalid := range []byte{2, 255} {
		if _, err := p.Modulate([]byte{1, invalid}); err == nil || p.phase != phase {
			t.Fatal("invalid input was accepted or advanced phase")
		}
	}
	if empty, err := p.Modulate(nil); err != nil || len(empty) != 0 || p.phase != phase {
		t.Fatal("empty input produced samples or advanced phase")
	}
}
