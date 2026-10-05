package css_16

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

var _ modulation.Processor = (*Processor)(nil)

func newTestProcessor(t *testing.T, rate, bitRate int) *Processor {
	t.Helper()
	p, err := New(modulation.Config{CarrierHz: 1070, SampleRate: rate, BitRate: bitRate})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// chirpCycles integrates the two linear segments in continuous time,
// independently of the implementation's sample-interval phase accumulator.
func chirpCycles(symbol int, seconds, duration float64) float64 {
	start := 820 + 500*float64(symbol)/16
	slope := 500 / duration
	wrap := duration * (1 - float64(symbol)/16)
	if seconds <= wrap {
		return start*seconds + slope*seconds*seconds/2
	}
	before := start*wrap + slope*wrap*wrap/2
	after := seconds - wrap
	return before + 820*after + slope*after*after/2
}

func TestModulate(t *testing.T) {
	bits := modulation.BytesToBits([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef})
	original := slices.Clone(bits)
	// 588 samples/symbol puts most wrap points between samples; four
	// samples/symbol exercises wraps within even the first interval.
	for _, timing := range [][2]int{{48000, 100}, {44100, 300}, {8000, 8000}} {
		t.Run(fmt.Sprint(timing), func(t *testing.T) {
			rate, bitRate := timing[0], timing[1]
			p := newTestProcessor(t, rate, bitRate)
			const initialPhase = 0.37
			p.phase = initialPhase
			got, err := p.Modulate(bits)
			if err != nil {
				t.Fatal(err)
			}
			spb := rate / bitRate
			if p.SampleRate() != rate || p.SamplesPerBit() != spb || len(got) != len(bits)*spb {
				t.Fatal("incorrect timing or sample count")
			}
			duration := 4.0 / float64(bitRate)
			for symbol := 0; symbol < 16; symbol++ {
				for j, sample := range got[symbol*4*spb : (symbol+1)*4*spb] {
					cycles := float64(symbol)*1070*duration + chirpCycles(symbol, float64(j)/float64(rate), duration)
					want := math.Sin(initialPhase + 2*math.Pi*cycles)
					if math.IsNaN(sample) || math.Abs(sample) > 1 || math.Abs(sample-want) > 1e-10 {
						t.Fatalf("symbol %d sample %d: got %g, want %g", symbol, j, sample, want)
					}
				}
			}
			chunked := newTestProcessor(t, rate, bitRate)
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
			if !slices.Equal(bits, original) {
				t.Fatal("input changed")
			}
			phase := p.phase
			for _, invalid := range [][]byte{{0}, {0, 1}, {0, 1, 0}, {0, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 2}, {255, 0, 0, 0}} {
				if _, err := p.Modulate(invalid); err == nil || p.phase != phase {
					t.Fatal("invalid input accepted or changed state")
				}
			}
			if empty, err := p.Modulate(nil); err != nil || len(empty) != 0 || p.phase != phase {
				t.Fatal("empty input produced samples or changed state")
			}
		})
	}
	var waveforms [][]float64
	for symbol := 0; symbol < 16; symbol++ {
		fresh := newTestProcessor(t, 48000, 100)
		waveform, err := fresh.Modulate(bits[4*symbol : 4*(symbol+1)])
		if err != nil || waveform[0] != 0 {
			t.Fatal("new processor has incorrect initial phase")
		}
		for _, previous := range waveforms {
			if slices.Equal(waveform, previous) {
				t.Fatal("different symbols produced identical waveforms")
			}
		}
		waveforms = append(waveforms, waveform)
	}
}

func TestInitializationAndOverflow(t *testing.T) {
	var p Processor
	if p.SampleRate() != 0 || p.SamplesPerBit() != 0 {
		t.Fatal("uninitialized processor reported timing")
	}
	for _, bits := range [][]byte{nil, {0, 0, 0, 0}} {
		if _, err := p.Modulate(bits); err == nil {
			t.Fatal("uninitialized processor accepted input")
		}
	}
	large := newTestProcessor(t, int(^uint(0)>>1), 1)
	if empty, err := large.Modulate(nil); err != nil || len(empty) != 0 || large.phase != 0 {
		t.Fatal("empty input overflowed symbol timing or changed state")
	}
	if _, err := large.Modulate([]byte{0, 0, 0, 0}); err == nil || large.phase != 0 {
		t.Fatal("overflow accepted or changed state")
	}
}
