package pm_bpsk

import (
	"math"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

var _ modulation.Processor = (*Processor)(nil)

func TestModulate(t *testing.T) {
	const spb = sampleRate / BitRate
	bits := []byte{0, 0, 1, 1, 0}
	original := slices.Clone(bits)
	// A nonzero start catches accidental resets, even with whole cycles per bit.
	const initialPhase = 0.37
	p := Processor{phase: initialPhase}
	got, err := p.Modulate(bits)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleRate() != sampleRate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
		t.Fatal("incorrect timing or sample count")
	}
	for i, sample := range got {
		want := math.Sin(initialPhase + 2*math.Pi*CarrierHz*float64(i)/sampleRate)
		if bits[i/spb] == 1 {
			want = -want
		}
		if math.IsNaN(sample) || math.Abs(sample) > 1 || math.Abs(sample-want) > 1e-11 {
			t.Fatalf("sample %d: got %g, want %g", i, sample, want)
		}
	}
	if !slices.Equal(bits, original) {
		t.Fatal("input changed")
	}
	chunked := Processor{phase: initialPhase}
	var joined []float64
	for _, chunk := range [][]byte{bits[:1], nil, bits[1:3], bits[3:]} {
		audio, err := chunked.Modulate(chunk)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, audio...)
	}
	if !slices.Equal(got, joined) {
		t.Fatal("chunk boundaries changed output")
	}
	var zero, one Processor
	positive, err := zero.Modulate([]byte{0})
	if err != nil {
		t.Fatal(err)
	}
	negative, err := one.Modulate([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	for i := range positive {
		if positive[i] != -negative[i] {
			t.Fatal("bit values must produce opposite waveforms")
		}
	}
	phase := p.phase
	for _, invalid := range []byte{2, 255} {
		if _, err := p.Modulate([]byte{0, invalid}); err == nil || p.phase != phase {
			t.Fatal("invalid input was accepted or advanced phase")
		}
	}
	if empty, err := p.Modulate(nil); err != nil || len(empty) != 0 || p.phase != phase {
		t.Fatal("empty input produced samples or advanced phase")
	}
}
