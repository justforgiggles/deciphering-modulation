package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"

	"github.com/justforgiggles/deciphering-modulation/fdm"
	"github.com/justforgiggles/deciphering-modulation/modulation"
)

const chunkSize = 256

// File frames carry sync bytes, a big-endian payload length, and a payload CRC32.
var framePreamble = [...]byte{0xa2, 0x3c, 0xee, 0xfc, 0xa7, 0x84, 0xdf, 0x4b, 0x38, 0x8c, 0xc4, 0xcb, 0x53, 0xa6, 0x91, 0xb1}

const frameOverhead = len(framePreamble) + 4 + 4

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

func encodeFile(inputPath, outputPath string, processor modulation.Processor) error {
	return encodeFiles([]string{inputPath}, outputPath, []modulation.Processor{processor})
}

func streamHeader(counts []int64, processors []modulation.Processor) (wavHeader, error) {
	if len(counts) < 1 || len(counts) > 2 || len(counts) != len(processors) {
		return wavHeader{}, errors.New("expected one or two matching inputs and processors")
	}
	longest := int64(0)
	for i, processor := range processors {
		if _, err := headerFor(counts[i], processor); err != nil {
			return wavHeader{}, err
		}
		if processor.SampleRate() != processors[0].SampleRate() || processor.SamplesPerBit() != processors[0].SamplesPerBit() {
			return wavHeader{}, errors.New("channel processors must have matching sample rates and samples per bit")
		}
		longest = max(longest, counts[i])
	}
	return headerFor(longest, processors[0])
}

func encodeFiles(inputPaths []string, outputPath string, processors []modulation.Processor) (err error) {
	inputs := make([]io.Reader, len(inputPaths))
	counts := make([]int64, len(inputPaths))
	for i, path := range inputPaths {
		input, err := os.Open(path)
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
		if info.Size() < 0 || info.Size() > math.MaxUint32 {
			return errors.New("input is too large for a framed WAV")
		}
		// Read once for the trailing CRC, then rewind for the streaming encoder.
		checksum := crc32.NewIEEE()
		if _, err := io.Copy(checksum, input); err != nil {
			return err
		}
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return err
		}
		var prefix [len(framePreamble) + 4]byte
		copy(prefix[:], framePreamble[:])
		binary.BigEndian.PutUint32(prefix[len(framePreamble):], uint32(info.Size()))
		var suffix [4]byte
		binary.BigEndian.PutUint32(suffix[:], checksum.Sum32())
		inputs[i] = io.MultiReader(bytes.NewReader(prefix[:]), input, bytes.NewReader(suffix[:]))
		counts[i] = info.Size() + int64(frameOverhead)
	}
	if _, err := streamHeader(counts, processors); err != nil {
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
	return encodeStreams(inputs, output, counts, processors)
}

func encodeAudio(input io.Reader, output io.Writer, byteCount int64, processor modulation.Processor) error {
	return encodeStreams([]io.Reader{input}, output, []int64{byteCount}, []modulation.Processor{processor})
}

func encodeStreams(inputs []io.Reader, output io.Writer, counts []int64, processors []modulation.Processor) error {
	if len(inputs) != len(counts) {
		return errors.New("input count mismatch")
	}
	header, err := streamHeader(counts, processors)
	if err != nil {
		return err
	}
	if err := binary.Write(output, binary.LittleEndian, header); err != nil {
		return err
	}
	longest := int64(0)
	for _, count := range counts {
		longest = max(longest, count)
	}
	var raw [chunkSize]byte
	for offset := int64(0); offset < longest; offset += chunkSize {
		channels := make([][]float64, len(inputs))
		for i, input := range inputs {
			n := int(min(max(counts[i]-offset, 0), chunkSize))
			if n == 0 {
				continue
			} // Ended channels contribute silence, not encoded zero bits.
			if _, err := io.ReadFull(input, raw[:n]); err != nil {
				return err
			}
			bits := modulation.BytesToBits(raw[:n])
			channels[i], err = processors[i].Modulate(bits)
			if err != nil {
				return err
			}
			if len(channels[i]) != len(bits)*processors[i].SamplesPerBit() {
				return errors.New("processor returned an unexpected number of samples")
			}
		}
		samples := channels[0]
		if len(channels) == 2 {
			samples, err = fdm.Mix(channels[0], channels[1])
			if err != nil {
				return err
			}
		}
		if err := writePCM(output, samples); err != nil {
			return err
		}
	}
	for _, input := range inputs {
		var extra [1]byte
		if _, err := io.ReadFull(input, extra[:]); err != io.EOF {
			if err != nil {
				return err
			}
			return errors.New("unexpected trailing input data")
		}
	}
	return nil
}

func writePCM(output io.Writer, samples []float64) error {
	pcm := make([]int16, len(samples))
	for i, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) || sample < -1 || sample > 1 {
			return fmt.Errorf("processor sample %d must be finite and in [-1, 1]", i)
		}
		pcm[i] = int16(math.Round(sample * 32767))
	}
	return binary.Write(output, binary.LittleEndian, pcm)
}
