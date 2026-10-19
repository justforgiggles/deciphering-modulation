package demodulation

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func wavBytes(samples []float64) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+2*len(samples)))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(48000), uint32(96000), uint16(2), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(2*len(samples)))
	for _, sample := range samples {
		binary.Write(&b, binary.LittleEndian, int16(math.Round(sample*32767)))
	}
	return b.Bytes()
}

func TestReadWAV(t *testing.T) {
	valid := wavBytes([]float64{-.5, 0, .5})
	metadata := append(bytes.Clone(valid[:12]), []byte("JUNK\x03\x00\x00\x00abc\x00")...)
	metadata = append(metadata, valid[12:]...)
	binary.LittleEndian.PutUint32(metadata[4:8], uint32(len(metadata)-8))
	path := filepath.Join(t.TempDir(), "input.wav")
	if err := os.WriteFile(path, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	audio, err := ReadAudioSamples(path)
	if err != nil || audio.SampleRate != 48000 || len(audio.Samples) != 3 || math.Abs(audio.Samples[0]+.5) > 1e-4 {
		t.Fatalf("valid WAV: %+v %v", audio, err)
	}
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"short", func(b []byte) []byte { return b[:10] }},
		{"RIFF size", func(b []byte) []byte { b[4]++; return b }},
		{"chunk overflow", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[40:44], math.MaxUint32); return b }},
		{"alignment", func(b []byte) []byte { b[32] = 4; return b }},
		{"byte rate", func(b []byte) []byte { b[28]++; return b }},
		{"stereo", func(b []byte) []byte { b[22] = 2; return b }},
		{"unsupported rate", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[24:28], 44100); return b }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, test.mutate(bytes.Clone(valid)), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadAudioSamples(path); err == nil {
				t.Fatal("accepted invalid/unsupported WAV")
			}
		})
	}
}

func TestAudioDurationLimit(t *testing.T) {
	// Sparse file exercises the length guard without constructing sample arrays.
	path := filepath.Join(t.TempDir(), "oversized.wav")
	dataBytes := uint32((SampleRate*MaxDurationSeconds + 1) * 2)
	header := wavBytes(nil)
	binary.LittleEndian.PutUint32(header[4:8], 36+dataBytes)
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(header); err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(int64(44 + dataBytes)); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadAudioSamples(path); err == nil || !strings.Contains(err.Error(), "300 seconds") {
		t.Fatal("duration limit not enforced", err)
	}
}

func TestConversionFailuresAndDirectWAV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(path, wavBytes([]float64{0, .5, -.5}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, cleanup, err := ConvertAudioToPCM(context.Background(), path)
	if err != nil || got != path {
		t.Fatal("compatible WAV should not need FFmpeg", err)
	}
	cleanup()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("cleanup removed original", err)
	}
	media := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(media, []byte("bad media"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ConvertAudioToPCM(context.Background(), media); err == nil || !strings.Contains(err.Error(), "FFmpeg required") {
		t.Fatal("missing FFmpeg error", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = ConvertAudioToPCM(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled conversion", err)
	}
}

func TestMP3Conversion(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg unavailable: MP3 conversion not tested")
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "source.wav")
	mp3 := filepath.Join(dir, "recording.mp3")
	payload := []byte("compressed audio")
	if err := os.WriteFile(wav, wavBytes(transmit(t, framed(payload), 6000).Samples), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("ffmpeg", "-v", "error", "-i", wav, "-c:a", "libmp3lame", mp3).CombinedOutput(); err != nil {
		t.Fatalf("create MP3: %v %s", err, output)
	}
	path, cleanup, err := ConvertAudioToPCM(context.Background(), mp3)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	audio, err := ReadAudioSamples(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := recoverFrames(context.Background(), audio)
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0].Payload, payload) {
		t.Fatalf("MP3 recovery: %+v %v", frames, err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temporary conversion was not removed")
	}
	bad := filepath.Join(dir, "bad.mp4")
	os.WriteFile(bad, []byte("invalid video"), 0600)
	if _, _, err := ConvertAudioToPCM(context.Background(), bad); err == nil {
		t.Fatal("accepted malformed media")
	}
}
