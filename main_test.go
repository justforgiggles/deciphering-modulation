package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

// This deliberately uses different timing from AM to check pipeline independence.
type fakeProcessor struct {
	bits     []byte
	calls    int
	modulate func([]byte) ([]float64, error)
}

func (*fakeProcessor) SampleRate() int    { return 8000 }
func (*fakeProcessor) SamplesPerBit() int { return 2 }
func (p *fakeProcessor) Modulate(bits []byte) ([]float64, error) {
	p.bits = append(p.bits, bits...)
	p.calls++
	if p.modulate != nil {
		return p.modulate(bits)
	}
	out := make([]float64, len(bits)*2)
	for i, bit := range bits {
		out[i*2] = float64(bit)
		out[i*2+1] = -float64(bit)
	}
	return out, nil
}

func TestWAVPipeline(t *testing.T) {
	data := bytes.Repeat([]byte{0x81}, chunkSize+3)
	for _, input := range [][]byte{nil, data} {
		var wav bytes.Buffer
		p := &fakeProcessor{}
		if err := encodeAudio(bytes.NewReader(input), &wav, int64(len(input)), p); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(p.bits, modulation.BytesToBits(input)) {
			t.Fatal("processor did not receive every bit in order")
		}
		if p.calls != (len(input)+chunkSize-1)/chunkSize {
			t.Fatal("incorrect chunk count")
		}
		reader := bytes.NewReader(wav.Bytes())
		var h wavHeader
		if err := binary.Read(reader, binary.LittleEndian, &h); err != nil {
			t.Fatal(err)
		}
		if string(h.RIFF[:]) != "RIFF" || string(h.WAVE[:]) != "WAVE" || string(h.FMT[:]) != "fmt " || string(h.DATA[:]) != "data" ||
			h.FormatSize != 16 || h.Format != 1 || h.Channels != 1 || h.SampleRate != 8000 || h.ByteRate != 16000 || h.BlockAlign != 2 || h.BitsPerSample != 16 ||
			int(h.DataSize) != len(input)*32 || int(h.Size) != wav.Len()-8 || wav.Len() != 44+len(input)*32 {
			t.Fatalf("incorrect PCM WAV header: %+v", h)
		}
		for _, bit := range p.bits {
			var pair [2]int16
			if err := binary.Read(reader, binary.LittleEndian, &pair); err != nil {
				t.Fatal(err)
			}
			if pair != [2]int16{int16(bit) * 32767, -int16(bit) * 32767} {
				t.Fatal("unexpected PCM samples")
			}
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestValidationAndIO(t *testing.T) {
	limit := (int64(math.MaxUint32) - 36) / (8 * int64((testProcessor(t, "am", 1070)).SamplesPerBit()) * 2)
	if _, err := headerFor(limit, testProcessor(t, "am", 1070)); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int64{-1, limit + 1, math.MaxInt64} {
		if _, err := headerFor(count, testProcessor(t, "am", 1070)); err == nil {
			t.Fatal("accepted invalid input size")
		}
	}
	if err := encodeAudio(bytes.NewReader(nil), io.Discard, 1, &fakeProcessor{}); !errors.Is(err, io.EOF) {
		t.Fatalf("missing input was not reported: %v", err)
	}
	if err := encodeAudio(bytes.NewReader([]byte{1}), io.Discard, 0, &fakeProcessor{}); err == nil {
		t.Fatal("ignored extra input")
	}
	if err := encodeAudio(bytes.NewReader([]byte{1}), failingWriter{}, 1, &fakeProcessor{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error was not propagated: %v", err)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1.01, 1.01} {
		p := &fakeProcessor{modulate: func(bits []byte) ([]float64, error) {
			samples := make([]float64, len(bits)*2)
			samples[0] = value
			return samples, nil
		}}
		if err := encodeAudio(bytes.NewReader([]byte{1}), io.Discard, 1, p); err == nil {
			t.Fatal("accepted invalid audio sample")
		}
	}
	p := &fakeProcessor{modulate: func([]byte) ([]float64, error) { return nil, nil }}
	if err := encodeAudio(bytes.NewReader([]byte{1}), io.Discard, 1, p); err == nil {
		t.Fatal("accepted wrong sample count")
	}
}

func demoInput(t *testing.T, data []byte) string {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.Mkdir("data", 0o755); err != nil {
		t.Fatal(err)
	}
	input := "data/image-small.png"
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return input
}

func runDemo(t *testing.T, kind string) string {
	t.Helper()
	before, err := filepath.Glob("output/*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{kind}); err != nil {
		t.Fatal(err)
	}
	after, err := filepath.Glob("output/*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatal("demo did not create a fresh WAV")
	}
	for _, path := range after {
		if !slices.Contains(before, path) {
			return path
		}
	}
	t.Fatal("demo did not create a fresh WAV")
	return ""
}

func TestFilesAndCLI(t *testing.T) {
	input := demoInput(t, []byte{0x80})
	output := runDemo(t, "am")
	before, err := os.ReadFile(output)
	if err != nil || len(before) != 44+8*(testProcessor(t, "am", 1070)).SamplesPerBit()*2 || binary.LittleEndian.Uint32(before[24:28]) != 48000 {
		t.Fatalf("incorrect AM WAV: %v", err)
	}
	if err := encodeFile(input, output, testProcessor(t, "am", 1070)); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing output was not protected: %v", err)
	}
	after, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing output changed")
	}
	partial := "partial.wav"
	p := &fakeProcessor{modulate: func([]byte) ([]float64, error) { return nil, io.ErrUnexpectedEOF }}
	if err := encodeFile(input, partial, p); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("processor error was not propagated: %v", err)
	}
	if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed conversion left partial output")
	}
	runDemo(t, "am")
	after, err = os.ReadFile(output)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("second run changed first output")
	}

}

func TestFMCLI(t *testing.T) {
	const spb = 48000 / 100
	demoInput(t, []byte{0x80})
	output := runDemo(t, "fm")
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)
	var header wavHeader
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		t.Fatal(err)
	}
	if len(data) != 44+8*spb*2 || header.DataSize != 8*spb*2 || header.SampleRate != 48000 ||
		float64(header.DataSize)/float64(header.ByteRate) != 8.0/100 {
		t.Fatal("incorrect FM WAV size or duration")
	}
	var pcm [8 * spb]int16
	if err := binary.Read(reader, binary.LittleEndian, &pcm); err != nil {
		t.Fatal(err)
	}
	// First bit selects the lower tone; the next retains its accumulated phase.
	if pcm[1] != int16(math.Round(math.Sin(2*math.Pi*(1070-250)/48000)*32767)) ||
		pcm[spb+1] != int16(math.Round(math.Sin(2*math.Pi*(1070-250)/100+2*math.Pi*(1070+250)/48000)*32767)) {
		t.Fatal("CLI did not generate continuous-phase FM")
	}
}

func TestPMCLI(t *testing.T) {
	const spb = 48000 / 100
	demoInput(t, []byte{0x80})
	output := runDemo(t, "pm_bpsk")
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)
	var header wavHeader
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		t.Fatal(err)
	}
	if len(data) != 44+8*spb*2 || header.DataSize != 8*spb*2 || header.SampleRate != 48000 ||
		float64(header.DataSize)/float64(header.ByteRate) != 8.0/100 {
		t.Fatal("incorrect PM WAV size or duration")
	}
	var pcm [8 * spb]int16
	if err := binary.Read(reader, binary.LittleEndian, &pcm); err != nil {
		t.Fatal(err)
	}
	// MSB-first: initial 1 inverts the carrier; the seven zeros do not.
	for i, sample := range pcm {
		want := math.Sin(2 * math.Pi * 1070 * float64(i) / 48000)
		if i < spb {
			want = -want
		}
		if math.Abs(float64(sample)-math.Round(want*32767)) > 1 {
			t.Fatalf("incorrect BPSK sample %d", i)
		}
	}
}

func TestQPSKCLI(t *testing.T) {
	const spb = 48000 / 100
	// All four phases, including a partial file chunk.
	source := bytes.Repeat([]byte{0x1e}, chunkSize+1)
	demoInput(t, source)
	output := runDemo(t, "pm_qpsk")
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)
	var header wavHeader
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		t.Fatal(err)
	}
	count := len(source) * 8 * spb
	if len(data) != 44+count*2 || int(header.DataSize) != count*2 || header.SampleRate != 48000 ||
		float64(header.DataSize)/float64(header.ByteRate) != float64(len(source)*8)/100 {
		t.Fatal("incorrect QPSK WAV size or duration")
	}
	pcm := make([]int16, count)
	if err := binary.Read(reader, binary.LittleEndian, pcm); err != nil {
		t.Fatal(err)
	}
	offsets := [4]float64{45, 135, 225, 315}
	for i, sample := range pcm {
		phase := 2*math.Pi*1070*float64(i)/48000 + offsets[(i/(2*spb))%4]*math.Pi/180
		if math.Abs(float64(sample)-math.Round(math.Sin(phase)*32767)) > 1 {
			t.Fatalf("incorrect QPSK sample %d", i)
		}
	}
}

func TestQAMCLI(t *testing.T) {
	const spb = 48000 / 100
	// Two amplitudes, including a partial file chunk.
	source := bytes.Repeat([]byte{0x89}, chunkSize+1)
	demoInput(t, source)
	output := runDemo(t, "qam")
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)
	var header wavHeader
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		t.Fatal(err)
	}
	count := len(source) * 8 * spb
	if len(data) != 44+count*2 || int(header.DataSize) != count*2 || header.SampleRate != 48000 ||
		float64(header.DataSize)/float64(header.ByteRate) != float64(len(source)*8)/100 {
		t.Fatal("incorrect QAM WAV size or duration")
	}
	pcm := make([]int16, count)
	if err := binary.Read(reader, binary.LittleEndian, pcm); err != nil {
		t.Fatal(err)
	}
	for i, sample := range pcm {
		q := -3.0
		if (i/(4*spb))%2 == 1 {
			q = -1
		}
		phase := 2 * math.Pi * 1070 * float64(i) / 48000
		want := (3*math.Cos(phase) - q*math.Sin(phase)) / math.Sqrt(18)
		if math.Abs(float64(sample)-math.Round(want*32767)) > 1 {
			t.Fatalf("incorrect QAM sample %d", i)
		}
	}
}

func TestCLIInvalidInput(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{nil, {"unknown"}, {"encode"}, {"am", "extra"}, {"fdm", "-in1", "file"}} {
		if err := run(args); err == nil {
			t.Fatalf("accepted invalid command %v", args)
		}
		if _, err := os.Stat("output"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid command created output")
		}
	}
	for _, kind := range []string{"am", "fdm"} {
		if err := run([]string{kind}); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing input: %v", err)
		}
	}
	files, err := os.ReadDir("output")
	if err != nil || len(files) != 0 {
		t.Fatal("missing input left output files")
	}
}
