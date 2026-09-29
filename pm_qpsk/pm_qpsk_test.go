package pm_qpsk

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
	for _, bits := range [][]byte{nil, {0, 0, 0, 0}} {
		if _, err := uninitialized.Modulate(bits); err == nil {
			t.Fatal("uninitialized processor accepted input")
		}
	}
	bits := []byte{0, 0, 0, 1, 1, 1, 1, 0, 1, 0}
	original := slices.Clone(bits)
	// An independent list locks down the Gray mapping, including repeated symbols.
	offsets := []float64{45, 135, 225, 315, 315}
	const initialPhase = 0.37
	p := newTestProcessor(t)
	p.phase = initialPhase
	got, err := p.Modulate(bits)
	if err != nil {
		t.Fatal(err)
	}
	spb := sampleRate / BitRate
	if p.SampleRate() != sampleRate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
		t.Fatal("incorrect timing or sample count")
	}
	for i, sample := range got {
		want := math.Sin(initialPhase + 2*math.Pi*CarrierHz*float64(i)/sampleRate + offsets[i/(2*spb)]*math.Pi/180)
		if math.IsNaN(sample) || math.Abs(sample) > 1 || math.Abs(sample-want) > 1e-10 {
			t.Fatalf("sample %d: got %g, want %g", i, sample, want)
		}
	}
	if !slices.Equal(bits, original) {
		t.Fatal("input changed")
	}
	chunked := newTestProcessor(t)
	chunked.phase = initialPhase
	var joined []float64
	for _, part := range [][]byte{bits[:2], nil, bits[2:6], bits[6:]} {
		audio, err := chunked.Modulate(part)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) || p.phase != chunked.phase {
		t.Fatal("chunk boundaries changed the signal or carrier phase")
	}
	phase := p.phase
	for _, invalid := range [][]byte{{0}, {0, 1, 0}, {0, 0, 0, 2}, {255, 0}} {
		if _, err := p.Modulate(invalid); err == nil || p.phase != phase {
			t.Fatal("invalid input accepted or changed state")
		}
	}
	if empty, err := p.Modulate(nil); err != nil || len(empty) != 0 || p.phase != phase {
		t.Fatal("empty chunk produced output or changed state")
	}
	fresh := newTestProcessor(t)
	first, err := fresh.Modulate([]byte{0, 0})
	if err != nil || math.Abs(first[0]-math.Sqrt(0.5)) > 1e-12 {
		t.Fatal("new processor has incorrect initial phase")
	}
}
