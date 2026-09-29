package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

var modulationKinds = []string{"am", "fm", "pm_bpsk", "pm_qpsk", "qam"}

func TestConfigurableCarriers(t *testing.T) {
	bits := modulation.BytesToBits([]byte{0x89})
	for _, kind := range modulationKinds {
		for _, carrier := range []float64{1070, 1375.5, 0, -100, 30000, math.MaxFloat64} {
			p, err := newProcessor(kind, &carrier)
			if err != nil {
				t.Fatal(err)
			}
			samples, err := p.Modulate(bits)
			if err != nil {
				t.Fatal(err)
			}
			for _, sample := range samples {
				if math.IsNaN(sample) || math.IsInf(sample, 0) || math.Abs(sample) > 1 {
					t.Fatal("invalid sample")
				}
			}
			phase := 2 * math.Pi * (carrier / float64(p.SampleRate()))
			want := math.Sin(phase)
			switch kind {
			case "fm":
				want = math.Sin(2 * math.Pi * ((carrier - 250) / float64(p.SampleRate())))
			case "pm_bpsk":
				want = -want
			case "pm_qpsk":
				want = math.Sin(math.Mod(phase, 2*math.Pi) + 7*math.Pi/4)
			case "qam":
				want = (3*math.Cos(math.Mod(phase, 2*math.Pi)) + 3*math.Sin(math.Mod(phase, 2*math.Pi))) / math.Sqrt(18)
			}
			// Extreme carriers are checked for finite output; sampled phase wraps modulo 2*pi.
			if math.Abs(carrier) < 1e6 && math.Abs(samples[1]-want) > 1e-12 {
				t.Fatalf("%s carrier %g ignored", kind, carrier)
			}
			if carrier == 1070 {
				zero, _ := newProcessor(kind, nil)
				original, _ := zero.Modulate(bits)
				if !slices.Equal(samples, original) {
					t.Fatalf("%s default changed", kind)
				}
			}
		}
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			if _, err := newProcessor(kind, &bad); err == nil {
				t.Fatal("accepted nonfinite carrier")
			}
		}
	}
}

func TestFDMStreams(t *testing.T) {
	for _, kind := range modulationKinds {
		t.Run(kind, func(t *testing.T) {
			for _, sizes := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, chunkSize + 1}, {chunkSize + 1, chunkSize - 1}} {
				data := [2][]byte{bytes.Repeat([]byte{0x89}, sizes[0]), bytes.Repeat([]byte{0x76}, sizes[1])}
				var processors []modulation.Processor
				var reference [2][]float64
				for i, carrier := range []float64{1070, 3070} {
					p, _ := newProcessor(kind, &carrier)
					processors = append(processors, p)
					whole, _ := newProcessor(kind, &carrier)
					reference[i], _ = whole.Modulate(modulation.BytesToBits(data[i]))
				}
				var wav bytes.Buffer
				err := encodeStreams([]io.Reader{bytes.NewReader(data[0]), bytes.NewReader(data[1])}, &wav, []int64{int64(sizes[0]), int64(sizes[1])}, processors)
				if err != nil {
					t.Fatal(err)
				}
				count := max(len(reference[0]), len(reference[1]))
				if wav.Len() != 44+2*count || int(binary.LittleEndian.Uint32(wav.Bytes()[40:44])) != 2*count {
					t.Fatal("wrong duration")
				}
				for i := 0; i < count; i++ {
					want := 0.0
					for _, channel := range reference {
						if i < len(channel) {
							want += channel[i] / 2
						}
					}
					got := int16(binary.LittleEndian.Uint16(wav.Bytes()[44+2*i:]))
					if got != int16(math.Round(want*32767)) {
						t.Fatalf("%s sizes %v sample %d: got %d want %g", kind, sizes, i, got, want)
					}
				}
			}
		})
	}
}

func TestFDMCLIAndErrors(t *testing.T) {
	dir := t.TempDir()
	first, second, out := filepath.Join(dir, "first"), filepath.Join(dir, "second"), filepath.Join(dir, "fdm.wav")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte{0xff}, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Identical carriers and files are allowed and combine to the original waveform.
	args := []string{"fdm", "-in1", first, "-in2", second, "-carrier1", "1070", "-carrier2", "1070", "-out", out}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := newProcessor("am", nil)
	var want bytes.Buffer
	if err := encodeAudio(bytes.NewReader([]byte{0xff}), &want, 1, p); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatal("identical carriers altered signal")
	}
	if err := run(args); err == nil {
		t.Fatal("overwrote output")
	}
	for _, bad := range [][]string{{"fdm"}, {"fdm", "-in1", first}, {"fdm", "-in1", first, "-in2", second, "-carrier2", "NaN"}, {"fdm", "-in1", first, "-in2", second, "-modulation", "unknown"}} {
		if err := run(bad); err == nil {
			t.Fatal("accepted invalid CLI")
		}
	}
	real, _ := newProcessor("am", nil)
	if _, err := streamHeader([]int64{1, 1}, []modulation.Processor{real, &fakeProcessor{}}); err == nil {
		t.Fatal("accepted mismatched timing")
	}
	for _, counts := range [][]int64{{1, 0}, {0, 1}, {-1, 0}, {math.MaxInt64, 0}} {
		if err := encodeStreams([]io.Reader{bytes.NewReader(nil), bytes.NewReader(nil)}, io.Discard, counts, []modulation.Processor{&fakeProcessor{}, &fakeProcessor{}}); err == nil {
			t.Fatal("accepted invalid size or truncated input")
		}
	}
	if err := encodeStreams([]io.Reader{bytes.NewReader(nil), bytes.NewReader([]byte{1})}, io.Discard, []int64{0, 0}, []modulation.Processor{&fakeProcessor{}, &fakeProcessor{}}); err == nil {
		t.Fatal("accepted trailing second input")
	}
	partial := filepath.Join(dir, "partial.wav")
	bad := &fakeProcessor{modulate: func([]byte) ([]float64, error) { return nil, io.ErrUnexpectedEOF }}
	if err := encodeFiles([]string{first, second}, partial, []modulation.Processor{&fakeProcessor{}, bad}); err == nil {
		t.Fatal("ignored second processor failure")
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("left partial output")
	}
}
