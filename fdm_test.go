package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

func testProcessor(t *testing.T, kind string, carrier float64) modulation.Processor {
	t.Helper()
	p, err := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 48000, BitRate: 100})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var modulationKinds = []string{"am", "fm", "pm_bpsk", "pm_qpsk", "qam", "css", "css_16"}

func TestConfigurableCarriers(t *testing.T) {
	bits := modulation.BytesToBits([]byte{0x89})
	for _, kind := range modulationKinds {
		for _, carrier := range []float64{1070, 1375.5, 0, -100, 30000, math.MaxFloat64} {
			p, err := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 48000, BitRate: 100})
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
			case "css_16":
				want = math.Sin(2 * math.Pi * (carrier + 250/float64(4*p.SamplesPerBit())) / float64(p.SampleRate()))
			case "css":
				want = math.Sin(2 * math.Pi * (carrier + 250 - 250/float64(p.SamplesPerBit())) / float64(p.SampleRate()))
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
				zero, _ := newProcessor(kind, modulation.Config{CarrierHz: 1070, SampleRate: 48000, BitRate: 100})
				original, _ := zero.Modulate(bits)
				if !slices.Equal(samples, original) {
					t.Fatalf("%s default changed", kind)
				}
			}
		}
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			if _, err := newProcessor(kind, modulation.Config{CarrierHz: bad, SampleRate: 48000, BitRate: 100}); err == nil {
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
					p, _ := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 48000, BitRate: 100})
					processors = append(processors, p)
					whole, _ := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 48000, BitRate: 100})
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
	first := demoInput(t, []byte{0xff})
	second := "data/lorem-ipsum-2.txt"
	if err := os.WriteFile(second, []byte{0x0f}, 0o600); err != nil {
		t.Fatal(err)
	}
	out := runDemo(t, "fdm")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	const spb = 480
	if len(got) != 44+(1+frameOverhead)*8*spb*2 || binary.LittleEndian.Uint32(got[24:28]) != 48000 {
		t.Fatal("incorrect FDM WAV size or rate")
	}
	for i := 0; i < 8*spb; i++ {
		want := math.Sin(2 * math.Pi * 1070 * float64(i) / 48000)
		if i >= 4*spb {
			want += math.Sin(2 * math.Pi * 3070 * float64(i) / 48000)
		}
		want /= 2
		sample := int16(binary.LittleEndian.Uint16(got[44+2*((len(framePreamble)+4)*8*spb+i):]))
		if math.Abs(float64(sample)-math.Round(want*32767)) > 1 {
			t.Fatalf("incorrect FDM sample %d", i)
		}
	}
	real, _ := newProcessor("am", modulation.Config{CarrierHz: 1070, SampleRate: 48000, BitRate: 100})
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
	partial := "partial.wav"
	bad := &fakeProcessor{modulate: func([]byte) ([]float64, error) { return nil, io.ErrUnexpectedEOF }}
	if err := encodeFiles([]string{first, second}, partial, []modulation.Processor{&fakeProcessor{}, bad}); err == nil {
		t.Fatal("ignored second processor failure")
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("left partial output")
	}
}

func TestProcessorConfiguration(t *testing.T) {
	for _, kind := range modulationKinds {
		t.Run(kind, func(t *testing.T) {
			for _, timing := range [][2]int{{48000, 100}, {8000, 200}, {44100, 300}, {8000, 8000}} {
				config := modulation.Config{CarrierHz: 1375.5, SampleRate: timing[0], BitRate: timing[1]}
				p, err := newProcessor(kind, config)
				if err != nil {
					t.Fatal(err)
				}
				spb := config.SampleRate / config.BitRate
				if p.SampleRate() != config.SampleRate || p.SamplesPerBit() != spb {
					t.Fatal("constructor ignored timing")
				}
				bits := modulation.BytesToBits([]byte{0x89, 0x76})
				got, err := p.Modulate(bits)
				if err != nil || len(got) != len(bits)*spb {
					t.Fatalf("incorrect sample count: %v", err)
				}
				chunked, err := newProcessor(kind, config)
				if err != nil {
					t.Fatal(err)
				}
				var joined []float64
				for _, chunk := range [][]byte{bits[:4], nil, bits[4:]} {
					part, err := chunked.Modulate(chunk)
					if err != nil {
						t.Fatal(err)
					}
					joined = append(joined, part...)
				}
				if !slices.Equal(got, joined) {
					t.Fatal("chunk boundaries changed output")
				}
				// Independently check the second sample, within the first symbol.
				if spb > 1 {
					phase := 2 * math.Pi * config.CarrierHz / float64(config.SampleRate)
					want := math.Sin(phase)
					switch kind {
					case "css_16":
						want = math.Sin(2 * math.Pi * (config.CarrierHz + 250/float64(4*spb)) / float64(config.SampleRate))
					case "css":
						want = math.Sin(2 * math.Pi * (config.CarrierHz + 250 - 250/float64(spb)) / float64(config.SampleRate))
					case "fm":
						want = math.Sin(2 * math.Pi * (config.CarrierHz - 250) / float64(config.SampleRate))
					case "pm_bpsk":
						want = -want
					case "pm_qpsk":
						want = math.Sin(phase + 7*math.Pi/4)
					case "qam":
						want = (3*math.Cos(phase) + 3*math.Sin(phase)) / math.Sqrt(18)
					}
					if math.Abs(got[1]-want) > 1e-12 {
						t.Fatal("waveform ignored configuration")
					}
				}
				var wav bytes.Buffer
				if err := encodeAudio(bytes.NewReader([]byte{0x89}), &wav, 1, p); err != nil {
					t.Fatal(err)
				}
				if wav.Len() != 44+8*spb*2 || int(binary.LittleEndian.Uint32(wav.Bytes()[24:28])) != config.SampleRate {
					t.Fatal("WAV ignored configured timing")
				}
			}
			for _, carrier := range []float64{math.MaxFloat64, -math.MaxFloat64} {
				p, err := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 1, BitRate: 1})
				if err != nil {
					t.Fatal(err)
				}
				samples, err := p.Modulate([]byte{0, 1, 1, 0})
				if err != nil {
					t.Fatal(err)
				}
				for _, sample := range samples {
					if math.IsNaN(sample) || math.IsInf(sample, 0) || math.Abs(sample) > 1 {
						t.Fatal("extreme carrier produced invalid sample")
					}
				}
			}
			large, err := newProcessor(kind, modulation.Config{CarrierHz: 1070, SampleRate: int(^uint(0) >> 1), BitRate: 1})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := large.Modulate([]byte{0, 0, 0, 0}); err == nil {
				t.Fatal("accepted overflowing sample count")
			}
			for _, timing := range [][2]int{{0, 100}, {-1, 100}, {48000, 0}, {48000, -1}, {48000, 101}, {100, 200}} {
				if _, err := newProcessor(kind, modulation.Config{CarrierHz: 1070, SampleRate: timing[0], BitRate: timing[1]}); err == nil {
					t.Fatalf("accepted invalid timing %v", timing)
				}
			}
			first, err := newProcessor(kind, modulation.Config{CarrierHz: 1070, SampleRate: 8000, BitRate: 100})
			if err != nil {
				t.Fatal(err)
			}
			for _, config := range []modulation.Config{
				{CarrierHz: 3070, SampleRate: 48000, BitRate: 100},
				{CarrierHz: 3070, SampleRate: 8000, BitRate: 200},
			} {
				second, err := newProcessor(kind, config)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := streamHeader([]int64{1, 1}, []modulation.Processor{first, second}); err == nil {
					t.Fatal("FDM accepted mismatched timing")
				}
			}
		})
	}
}
