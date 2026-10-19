package demodulation

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/justforgiggles/deciphering-modulation/css"
	"github.com/justforgiggles/deciphering-modulation/modulation"
)

func framed(payload []byte) []byte {
	frame := append([]byte(nil), preamble[:]...)
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)
	return binary.BigEndian.AppendUint32(frame, crc32.ChecksumIEEE(payload))
}

func transmit(t *testing.T, frame []byte, carrier float64) Audio {
	t.Helper()
	processor, err := css.New(modulation.Config{CarrierHz: carrier, SampleRate: SampleRate, BitRate: 100})
	if err != nil {
		t.Fatal(err)
	}
	samples, err := processor.Modulate(modulation.BytesToBits(frame))
	if err != nil {
		t.Fatal(err)
	}
	return Audio{samples, SampleRate}
}

func prepare(audio Audio, config Config) IQ {
	centered := RemoveDCOffset(audio)
	carrier := GenerateQuadratureCarrier(len(audio.Samples), audio.SampleRate, config.CarrierHz)
	mixed := MixToBaseband(centered, carrier)
	factor := DownsampleFactor(audio.SampleRate, config)
	return Downsample(ApplyFIRFilter(mixed, DesignLowPassFilter(audio.SampleRate, factor)), factor)
}

func recoverFrames(ctx context.Context, audio Audio) ([]VerifiedFrame, error) {
	c := DefaultConfig()
	x := prepare(audio, c)
	candidates, err := FindPreambleCandidates(ctx, x, c)
	if err != nil {
		return nil, err
	}
	var frames []VerifiedFrame
	var failure error
	for _, candidate := range candidates {
		if len(frames) > 0 && candidate.StartSample/x.SampleRate < frames[len(frames)-1].EndSeconds {
			continue
		}
		sync := RefineSynchronization(x, c, candidate)
		decisions, err := DetectSymbolsWithTracking(ctx, x, c, sync)
		var frame VerifiedFrame
		if err == nil {
			frame, err = AssembleAndValidateFrame(decisions, sync.SamplesPerSymbol/x.SampleRate)
		}
		if err != nil {
			failure = err
			continue
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return nil, failure
	}
	return frames, nil
}

func TestReceiver(t *testing.T) {
	payload := []byte("CSS: \x00\xff\x80\n")
	for _, test := range []struct {
		name                        string
		payload                     []byte
		carrier, scale, gain, noise float64
	}{
		{"binary", payload, 6000, 1, 1, 0},
		{"empty", nil, 6000, 1, 1, 0},
		{"noise echo fractional timing", payload, 6000, 1, .1, .01},
		{"slow clock", payload, 6000, 1.003, .1, .005},
		{"fast clock and frequency offset", payload, 6035, .997, .1, .005},
	} {
		t.Run(test.name, func(t *testing.T) {
			audio := transmit(t, framed(test.payload), test.carrier)
			if test.gain != 1 {
				source := audio.Samples
				at := func(position float64) float64 {
					i := int(math.Floor(position))
					f := position - float64(i)
					if i < 0 || i+1 >= len(source) {
						return 0
					}
					return source[i]*(1-f) + source[i+1]*f
				}
				random := rand.New(rand.NewPCG(12, 34))
				audio.Samples = make([]float64, 1732+int(float64(len(source))*test.scale)+4800)
				for i := range audio.Samples {
					position := (float64(i) - 1731.4) / test.scale
					audio.Samples[i] = .03 + test.gain*(at(position)+.08*at(position-72)) + test.noise*random.NormFloat64()
				}
			}
			frames, err := recoverFrames(context.Background(), audio)
			if err != nil || len(frames) != 1 || !bytes.Equal(frames[0].Payload, test.payload) {
				t.Fatalf("recovery: %+v, %v", frames, err)
			}
		})
	}
}

func TestChangingFrequency(t *testing.T) {
	payload := bytes.Repeat([]byte{0x93, 0x08, 0xfe, 0x37}, 8)
	bits := modulation.BytesToBits(framed(payload))
	// Independently integrate a drifting passband carrier, retaining its phase.
	audio := Audio{make([]float64, len(bits)*480+4800), 48000}
	phase := .73
	for i := 0; i < len(bits)*480; i++ {
		bit := bits[i/480]
		frequency := 6000 - 60 + 150*float64(i)/float64(len(bits)*480) + (1-2*float64(bit))*500*((float64(i%480)+.5)/480-.5)
		audio.Samples[i] = .2 * math.Sin(phase)
		phase += 2 * math.Pi * frequency / 48000
	}
	frames, err := recoverFrames(context.Background(), audio)
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0].Payload, payload) {
		t.Fatalf("frequency tracking: %+v, %v", frames, err)
	}
}

func TestRejectedFramesAndLaterRecovery(t *testing.T) {
	good := framed([]byte("payload"))
	bad := bytes.Clone(good)
	bad[len(bad)-1] ^= 1
	for _, test := range []struct {
		name  string
		audio Audio
	}{
		{"checksum", transmit(t, bad, 6000)},
		{"truncated", transmit(t, good[:len(good)-2], 6000)},
		{"silence", Audio{make([]float64, 48000), 48000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, err := recoverFrames(context.Background(), test.audio)
			if err == nil || len(frames) != 0 {
				t.Fatalf("accepted damaged input: %+v, %v", frames, err)
			}
		})
	}
	source := transmit(t, good, 6000)
	var damaged []float64
	for bit := 0; bit < len(source.Samples)/480; bit++ {
		if bit%8 != 3 {
			damaged = append(damaged, source.Samples[bit*480:(bit+1)*480]...)
		}
	}
	if frames, err := recoverFrames(context.Background(), Audio{damaged, 48000}); err == nil || len(frames) != 0 {
		t.Fatal("accepted dropped audio")
	}
	audio := transmit(t, bad, 6000)
	audio.Samples = append(audio.Samples, make([]float64, 4800)...)
	audio.Samples = append(audio.Samples, source.Samples...)
	frames, err := recoverFrames(context.Background(), audio)
	if err != nil || len(frames) != 1 || string(frames[0].Payload) != "payload" {
		t.Fatalf("later valid frame: %+v %v", frames, err)
	}
}

func TestFrameValidation(t *testing.T) {
	toDecisions := func(data []byte) []SymbolDecision {
		var decisions []SymbolDecision
		for _, bit := range modulation.BytesToBits(data) {
			decisions = append(decisions, SymbolDecision{Bit: bit})
		}
		return decisions
	}
	good := framed([]byte{0x00, 0x01, 0x80, 0xff})
	frame, err := AssembleAndValidateFrame(toDecisions(good), .01)
	if err != nil || !bytes.Equal(frame.Payload, []byte{0, 1, 128, 255}) {
		t.Fatal("bit ordering", err)
	}
	bad := bytes.Clone(good)
	binary.BigEndian.PutUint32(bad[8:12], math.MaxUint32)
	if _, err := AssembleAndValidateFrame(toDecisions(bad), .01); !errors.Is(err, ErrTruncated) {
		t.Fatal("accepted impossible length", err)
	}
	bad = bytes.Clone(good)
	bad[len(bad)-1] ^= 1
	if _, err := AssembleAndValidateFrame(toDecisions(bad), .01); !errors.Is(err, ErrChecksum) {
		t.Fatal("accepted corrupt CRC", err)
	}
}

func TestCancellationAndOutputProtection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FindPreambleCandidates(ctx, IQ{make([]float64, 3000), make([]float64, 3000), 3000}, DefaultConfig()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "results")
	frames := []VerifiedFrame{{Payload: []byte("first")}, {Payload: []byte("second")}}
	if err := WriteVerifiedPayloads(ctx, dir, frames); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("cancelled operation created output")
	}
	if err := WriteVerifiedPayloads(context.Background(), dir, frames); err != nil {
		t.Fatal(err)
	}
	if err := WriteVerifiedPayloads(context.Background(), dir, frames); !os.IsExist(err) {
		t.Fatal("existing output not protected", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "frame-001.bin"))
	if err != nil || string(got) != "first" {
		t.Fatal("output changed", err)
	}
}
