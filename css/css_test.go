package css

import (
	"math"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

const (
	CarrierHz  = 1070
	BitRate    = 100
	sampleRate = 48000
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
	for _, bits := range [][]byte{nil, {0, 1}} {
		if _, err := uninitialized.Modulate(bits); err == nil {
			t.Fatal("uninitialized processor accepted input")
		}
	}
	const spb = sampleRate / BitRate
	const initialPhase = 0.37
	bits := []byte{0, 1, 1, 0, 1, 0, 0}
	original := slices.Clone(bits)
	p := newTestProcessor(t)
	p.phase = initialPhase
	got, err := p.Modulate(bits)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleRate() != sampleRate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
		t.Fatal("incorrect timing or sample count")
	}
	// Closed-form integral: f_start*t + slope*t²/2. Each complete
	// chirp accumulates CarrierHz/BitRate cycles, regardless of direction.
	for i, bit := range bits {
		startHz, slope := 820.0, 500.0*BitRate
		if bit == 1 {
			startHz, slope = 1320, -500*BitRate
		}
		for j, sample := range got[i*spb : (i+1)*spb] {
			seconds := float64(j) / sampleRate
			cycles := float64(i)*CarrierHz/BitRate + startHz*seconds + slope*seconds*seconds/2
			want := math.Sin(initialPhase + 2*math.Pi*cycles)
			if math.IsNaN(sample) || math.Abs(sample) > 1 || math.Abs(sample-want) > 1e-11 {
				t.Fatalf("bit %d sample %d: got %g, want %g", i, j, sample, want)
			}
		}
	}
	if !slices.Equal(bits, original) {
		t.Fatal("input changed")
	}
	chunked := newTestProcessor(t)
	chunked.phase = initialPhase
	var joined []float64
	for _, chunk := range [][]byte{bits[:1], nil, bits[1:4], bits[4:]} {
		audio, err := chunked.Modulate(chunk)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) || p.phase != chunked.phase {
		t.Fatal("chunk boundaries changed waveform or state")
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
	fresh := newTestProcessor(t)
	first, err := fresh.Modulate([]byte{0})
	if err != nil || first[0] != 0 {
		t.Fatal("new processor has incorrect initial phase")
	}
}
