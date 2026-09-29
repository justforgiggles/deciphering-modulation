package am

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
	const spb = sampleRate / BitRate
	bits := []byte{0, 1, 0, 1, 1}
	original := slices.Clone(bits)
	p := newTestProcessor(t)
	got, err := p.Modulate(bits)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleRate() != sampleRate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
		t.Fatal("incorrect timing or sample count")
	}
	for i, sample := range got {
		want := float64(bits[i/spb]) * math.Sin(2*math.Pi*CarrierHz*float64(i)/sampleRate)
		if math.Abs(sample-want) > 1e-12 {
			t.Fatalf("sample %d: got %g, want %g", i, sample, want)
		}
	}
	if !slices.Equal(bits, original) {
		t.Fatal("input changed")
	}

	chunked := newTestProcessor(t)
	var joined []float64
	for _, chunk := range [][]byte{bits[:1], nil, bits[1:3], bits[3:]} {
		audio, err := chunked.Modulate(chunk)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) {
		t.Fatal("chunk boundaries changed the output")
	}

	// Start between cycles to check that chunks do not reset carrier state.
	offset := newTestProcessor(t)
	offset.phase = 0.37
	audio, err := offset.Modulate([]byte{0, 1})
	if err != nil || math.Abs(audio[spb]-math.Sin(0.37+2*math.Pi*CarrierHz*spb/sampleRate)) > 1e-12 {
		t.Fatal("carrier state was reset or lost during silence")
	}
	phase := offset.phase
	for _, invalid := range []byte{2, 255} {
		if _, err := offset.Modulate([]byte{1, invalid}); err == nil || offset.phase != phase {
			t.Fatal("invalid chunk was accepted or changed carrier state")
		}
	}
	if empty, err := offset.Modulate(nil); err != nil || len(empty) != 0 || offset.phase != phase {
		t.Fatal("empty chunk changed state or produced output")
	}
}
