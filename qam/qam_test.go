package qam

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
	// Explicit constellation in binary symbol order, independently of the lookup.
	points := [16][2]float64{
		{-3, -3}, {-3, -1}, {-3, 3}, {-3, 1},
		{-1, -3}, {-1, -1}, {-1, 3}, {-1, 1},
		{3, -3}, {3, -1}, {3, 3}, {3, 1},
		{1, -3}, {1, -1}, {1, 3}, {1, 1},
	}
	var bits []byte
	for symbol := 0; symbol < 16; symbol++ {
		for shift := 3; shift >= 0; shift-- {
			bits = append(bits, byte(symbol>>shift)&1)
		}
	}
	original := slices.Clone(bits)
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
		point := points[i/(4*spb)]
		// Polar form is an independent check of the I/Q mixer and normalization.
		radius := math.Hypot(point[0], point[1]) / math.Sqrt(18)
		phase := initialPhase + 2*math.Pi*CarrierHz*float64(i)/sampleRate + math.Atan2(point[1], point[0])
		want := radius * math.Cos(phase)
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
	for _, part := range [][]byte{bits[:4], nil, bits[4:28], bits[28:]} {
		audio, err := chunked.Modulate(part)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) || p.phase != chunked.phase {
		t.Fatal("chunk boundaries changed waveform or state")
	}
	phase := p.phase
	for _, invalid := range [][]byte{{0}, {0, 1}, {0, 1, 0}, {0, 0, 0, 0, 0}, {0, 0, 0, 2}, {255, 0, 0, 0}} {
		if _, err := p.Modulate(invalid); err == nil || p.phase != phase {
			t.Fatal("invalid input accepted or changed state")
		}
	}
	if empty, err := p.Modulate(nil); err != nil || len(empty) != 0 || p.phase != phase {
		t.Fatal("empty input produced samples or changed state")
	}
	fresh := newTestProcessor(t)
	first, err := fresh.Modulate([]byte{0, 0, 0, 0})
	if err != nil || math.Abs(first[0]+1/math.Sqrt2) > 1e-12 {
		t.Fatal("new processor has incorrect initial phase")
	}
}
