package demodulation

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const SampleRate = 48000
const MaxDurationSeconds = 300
const maxWAVBytes = 64 << 20

var ErrUnsupportedWAV = errors.New("expected mono 48 kHz 16-bit PCM WAV")

// Audio holds normalized real samples: one value per sample time, not per bit.
type Audio struct {
	Samples    []float64
	SampleRate float64
}

// ConvertAudioToPCM preserves compatible WAVs. Other media is decoded by FFmpeg
// into a private temporary WAV. Always call the returned cleanup on success.
// No shell, gain adjustment, silence removal, or time stretching is involved.
func ConvertAudioToPCM(ctx context.Context, input string) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	path, err := filepath.Abs(input)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("input must be a regular file")
	}
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		_, err := ReadAudioSamples(path)
		if err == nil {
			return path, func() {}, nil
		}
		if !errors.Is(err, ErrUnsupportedWAV) {
			return "", nil, err
		}
	}
	dir, err := os.MkdirTemp("", "css-pcm-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	output := filepath.Join(dir, "audio.wav")
	// Decode one second beyond the limit so an oversized input is rejected,
	// rather than silently accepting a truncated 300-second recording.
	command := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error",
		"-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:a:0", "-vn",
		"-ac", "1", "-ar", "48000", "-c:a", "pcm_s16le", "-t", "301", "-f", "wav", output)
	var diagnostic limitedDiagnostic
	command.Stderr = &diagnostic
	if err := command.Run(); err != nil {
		cleanup()
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, fmt.Errorf("convert audio (FFmpeg required): %w: %s", err, diagnostic.text)
	}
	if _, err := ReadAudioSamples(output); err != nil {
		cleanup()
		return "", nil, err
	}
	return output, cleanup, nil
}

// Bound subprocess diagnostics without stopping it from draining stderr.
type limitedDiagnostic struct{ text []byte }

func (d *limitedDiagnostic) Write(p []byte) (int, error) {
	n := min(len(p), 8192-len(d.text))
	d.text = append(d.text, p[:n]...)
	return len(p), nil
}

// ReadAudioSamples walks RIFF chunks (including metadata and odd-byte padding).
// A WAV header is not always 44 bytes. All lengths are checked before slicing.
func ReadAudioSamples(path string) (Audio, error) {
	f, err := os.Open(path)
	if err != nil {
		return Audio{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxWAVBytes+1))
	if err != nil {
		return Audio{}, err
	}
	if len(b) > maxWAVBytes {
		return Audio{}, errors.New("WAV exceeds 64 MiB limit")
	}
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return Audio{}, errors.New("invalid RIFF/WAVE header or size")
	}
	var format, pcm []byte
	for position := 12; position < len(b); {
		if len(b)-position < 8 {
			return Audio{}, errors.New("truncated WAV chunk header")
		}
		size := uint64(binary.LittleEndian.Uint32(b[position+4 : position+8]))
		end := uint64(position) + 8 + size
		padded := end + size%2
		if padded > uint64(len(b)) {
			return Audio{}, errors.New("truncated WAV chunk")
		}
		switch string(b[position : position+4]) {
		case "fmt ":
			if format != nil {
				return Audio{}, errors.New("duplicate WAV format")
			}
			format = b[position+8 : int(end)]
		case "data":
			if pcm != nil {
				return Audio{}, errors.New("duplicate WAV data")
			}
			pcm = b[position+8 : int(end)]
		}
		position = int(padded)
	}
	if len(format) < 16 || pcm == nil {
		return Audio{}, errors.New("missing WAV format or data")
	}
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	if u16(format[:2]) != 1 || u16(format[2:4]) != 1 || u32(format[4:8]) != SampleRate || u16(format[14:16]) != 16 {
		return Audio{}, ErrUnsupportedWAV
	}
	if u32(format[8:12]) != SampleRate*2 || u16(format[12:14]) != 2 || len(pcm)%2 != 0 {
		return Audio{}, errors.New("invalid WAV byte rate or sample alignment")
	}
	if len(pcm)/2 > SampleRate*MaxDurationSeconds {
		return Audio{}, errors.New("audio exceeds 300 seconds")
	}
	if len(pcm) == 0 {
		return Audio{}, errors.New("empty audio")
	}
	x := make([]float64, len(pcm)/2)
	for i := range x {
		x[i] = float64(int16(u16(pcm[2*i:2*i+2]))) / 32768
	}
	return Audio{Samples: x, SampleRate: SampleRate}, nil
}
