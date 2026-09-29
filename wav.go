package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/justforgiggles/deciphering-modulation/modulation"
)

const chunkSize = 256

// wavHeader describes the canonical mono, 16-bit PCM format we write.
type wavHeader struct {
	RIFF          [4]byte
	Size          uint32
	WAVE          [4]byte
	FMT           [4]byte
	FormatSize    uint32
	Format        uint16
	Channels      uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
	DATA          [4]byte
	DataSize      uint32
}

func headerFor(byteCount int64, processor modulation.Processor) (wavHeader, error) {
	rate, samplesPerBit := int64(processor.SampleRate()), int64(processor.SamplesPerBit())
	if rate <= 0 || rate > math.MaxUint32/2 || samplesPerBit <= 0 {
		return wavHeader{}, errors.New("invalid processor timing for PCM WAV")
	}
	// Divide before multiplying to reject oversized input without integer overflow.
	if byteCount < 0 || byteCount > (int64(math.MaxUint32)-36)/2/8/samplesPerBit {
		return wavHeader{}, errors.New("input is too large for a standard RIFF WAV file at this bit rate")
	}
	dataSize := uint32(byteCount * 8 * samplesPerBit * 2)
	return wavHeader{
		RIFF: [4]byte{'R', 'I', 'F', 'F'}, Size: 36 + dataSize,
		WAVE: [4]byte{'W', 'A', 'V', 'E'}, FMT: [4]byte{'f', 'm', 't', ' '},
		FormatSize: 16, Format: 1, Channels: 1, SampleRate: uint32(rate),
		ByteRate: uint32(rate * 2), BlockAlign: 2, BitsPerSample: 16,
		DATA: [4]byte{'d', 'a', 't', 'a'}, DataSize: dataSize,
	}, nil
}

func encodeFile(inputPath, outputPath string, processor modulation.Processor) (err error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("input must be a regular file")
	}
	if _, err := headerFor(info.Size(), processor); err != nil {
		return err
	}
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, output.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(outputPath))
		}
	}()
	return encodeAudio(input, output, info.Size(), processor)
}

func encodeAudio(input io.Reader, output io.Writer, byteCount int64, processor modulation.Processor) error {
	header, err := headerFor(byteCount, processor)
	if err != nil {
		return err
	}
	if err := binary.Write(output, binary.LittleEndian, header); err != nil {
		return err
	}
	var raw [chunkSize]byte
	for remaining := byteCount; remaining > 0; {
		n := int(min(remaining, chunkSize))
		if _, err := io.ReadFull(input, raw[:n]); err != nil {
			return err
		}
		bits := modulation.BytesToBits(raw[:n])
		samples, err := processor.Modulate(bits)
		if err != nil {
			return err
		}
		if len(samples) != len(bits)*processor.SamplesPerBit() {
			return errors.New("processor returned an unexpected number of samples")
		}
		pcm := make([]int16, len(samples))
		for i, sample := range samples {
			if math.IsNaN(sample) || math.IsInf(sample, 0) || sample < -1 || sample > 1 {
				return fmt.Errorf("processor sample %d must be finite and in [-1, 1]", i)
			}
			pcm[i] = int16(math.Round(sample * 32767))
		}
		if err := binary.Write(output, binary.LittleEndian, pcm); err != nil {
			return err
		}
		remaining -= int64(n)
	}
	// Detect input growth after Stat instead of silently ignoring extra bytes.
	var extra [1]byte
	if _, err := io.ReadFull(input, extra[:]); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("unexpected trailing input data")
	}
	return nil
}
