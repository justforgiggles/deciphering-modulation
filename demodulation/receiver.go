package demodulation

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"sort"
)

var preamble = [...]byte{0xa2, 0x3c, 0xee, 0xfc, 0xa7, 0x84, 0xdf, 0x4b}
var (
	ErrNoPreamble = errors.New("no CSS preamble found")
	ErrTruncated  = errors.New("truncated CSS frame")
	ErrChecksum   = errors.New("CSS checksum mismatch")
)

// Synchronization uses positions and periods in BASEBAND samples, not PCM
// samples. OffsetHz is relative to the nominal carrier used by the mixer.
type Synchronization struct{ StartSample, SamplesPerSymbol, OffsetHz, PreambleScore float64 }

type SymbolDecision struct {
	Bit                                           byte
	StartSeconds, OffsetHz, SoftEvidence, Quality float64
}

// VerifiedFrame is constructed only after both structure and CRC checks pass.
// Confidence is mean absolute soft evidence, not a probability of correctness.
type VerifiedFrame struct {
	Payload                              []byte
	StartSeconds, EndSeconds, Confidence float64
}

type symbolEvidence struct{ soft, quality float64 }

func compareChirps(window, up, down IQ) symbolEvidence {
	upEnergy := MeasureCoherentEnergy(Dechirp(window, up))
	downEnergy := MeasureCoherentEnergy(Dechirp(window, down))
	var inputEnergy float64
	for i := range window.I {
		inputEnergy += window.I[i]*window.I[i] + window.Q[i]*window.Q[i]
	}
	if inputEnergy < 1e-24 || upEnergy+downEnergy < 1e-24 {
		return symbolEvidence{}
	}
	return symbolEvidence{
		soft:    (downEnergy - upEnergy) / (downEnergy + upEnergy),
		quality: math.Max(upEnergy, downEnergy) / (float64(len(window.I)) * inputEnergy),
	}
}

func preambleSign(bit int) float64 { return 2*float64((preamble[bit/8]>>uint(7-bit%8))&1) - 1 }

// FindPreambleCandidates compares soft up/down evidence with the known 64 bits.
// We cache each sample-position score for a frequency hypothesis so the nested
// timing search reuses evidence; it does not use any expected payload bytes.
func FindPreambleCandidates(ctx context.Context, input IQ, config Config) ([]Synchronization, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if len(input.I) != len(input.Q) || input.SampleRate <= 0 {
		return nil, errors.New("invalid I/Q signal")
	}
	nominal := input.SampleRate / config.BitRate
	if nominal < 12 || nominal > float64(len(input.I))/64 {
		return nil, ErrNoPreamble
	}
	var candidates []Synchronization
	for offset := -200.0; offset <= 200; offset += 25 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		up := GenerateReferenceChirp(nominal, input.SampleRate, config.BandwidthHz, offset, 0)
		down := GenerateReferenceChirp(nominal, input.SampleRate, config.BandwidthHz, offset, 1)
		scores := make([]float64, len(input.I)-len(up.I)+1)
		for i := range scores {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			window := IQ{input.I[i : i+len(up.I)], input.Q[i : i+len(up.I)], input.SampleRate}
			scores[i] = compareChirps(window, up, down).soft
		}
		for drift := -10; drift <= 10; drift++ {
			period := nominal * (1 + float64(drift)*.001)
			last := len(scores) - 1 - int(math.Round(63*period))
			for start := 0; start <= last; start += max(1, int(nominal/12)) {
				var sum float64
				for bit := 0; bit < 64; bit++ {
					sum += preambleSign(bit) * scores[start+int(math.Round(float64(bit)*period))]
				}
				score := sum / 64
				if score < .65 {
					continue
				}
				candidate := Synchronization{float64(start), period, offset, score}
				merged := false
				for i, previous := range candidates {
					if math.Abs(previous.StartSample-candidate.StartSample) < nominal {
						if score > previous.PreambleScore {
							candidates[i] = candidate
						}
						merged = true
						break
					}
				}
				if !merged {
					candidates = append(candidates, candidate)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoPreamble
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].StartSample < candidates[j].StartSample })
	return candidates, nil
}

func synchronizationScore(input IQ, config Config, candidate Synchronization) float64 {
	up := GenerateReferenceChirp(candidate.SamplesPerSymbol, input.SampleRate, config.BandwidthHz, candidate.OffsetHz, 0)
	down := GenerateReferenceChirp(candidate.SamplesPerSymbol, input.SampleRate, config.BandwidthHz, candidate.OffsetHz, 1)
	var sum float64
	for bit := 0; bit < 64; bit++ {
		window, ok := InterpolateSymbol(input, candidate.StartSample+float64(bit)*candidate.SamplesPerSymbol, len(up.I))
		if !ok {
			return -1
		}
		sum += preambleSign(bit) * compareChirps(window, up, down).soft
	}
	return sum / 64
}

// RefineSynchronization adjusts start, period, and frequency one axis at a
// time, then shrinks the step sizes. This is an explicit local search rather
// than an opaque optimizer. Coarse acquisition supplies the starting point.
func RefineSynchronization(input IQ, config Config, candidate Synchronization) Synchronization {
	steps := []float64{candidate.SamplesPerSymbol / 12, candidate.SamplesPerSymbol * .001, 12.5}
	candidate.PreambleScore = synchronizationScore(input, config, candidate)
	for pass := 0; pass < 12; pass++ {
		for axis, step := range steps {
			best := candidate
			for _, direction := range []float64{-1, 1} {
				trial := candidate
				switch axis {
				case 0:
					trial.StartSample += direction * step
				case 1:
					trial.SamplesPerSymbol += direction * step
				case 2:
					trial.OffsetHz += direction * step
				}
				if math.Abs(trial.OffsetHz) > 200 {
					continue
				}
				trial.PreambleScore = synchronizationScore(input, config, trial)
				if trial.PreambleScore > best.PreambleScore {
					best = trial
				}
			}
			candidate = best
		}
		for i := range steps {
			steps[i] *= .65
		}
	}
	return candidate
}

// DetectSymbolsWithTracking searches residual frequency for EACH symbol.
// A single preamble frequency estimate can become wrong while the sound plays.
// Early/late windows also follow timing, with a slow period adjustment to keep
// a small sample-clock error from accumulating into an entire symbol slip.
func DetectSymbolsWithTracking(ctx context.Context, input IQ, config Config, sync Synchronization) ([]SymbolDecision, error) {
	period, position := sync.SamplesPerSymbol, sync.StartSample
	count := int(math.Round(period))
	// Build actual reference sample arrays once for each frequency hypothesis.
	type referencePair struct {
		offset   float64
		up, down IQ
	}
	var references []referencePair
	for offset := -200.0; offset <= 200; offset += 5 {
		references = append(references, referencePair{offset,
			GenerateReferenceChirp(period, input.SampleRate, config.BandwidthHz, offset, 0),
			GenerateReferenceChirp(period, input.SampleRate, config.BandwidthHz, offset, 1)})
	}
	var decisions []SymbolDecision
	totalBits := 96 // First recover just the preamble and length.
	for bit := 0; bit < totalBits; bit++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		best := symbolEvidence{quality: -1}
		bestShift, bestOffset := 0.0, 0.0
		// The ±5% timing neighborhood scales with samples per symbol.
		for step := -6; step <= 6; step++ {
			shift := float64(step) * sync.SamplesPerSymbol / 120
			window, ok := InterpolateSymbol(input, position+shift, count)
			if !ok {
				continue
			}
			for _, reference := range references {
				evidence := compareChirps(window, reference.up, reference.down)
				if evidence.quality > best.quality {
					best = evidence
					bestShift = shift
					bestOffset = reference.offset
				}
			}
		}
		if best.quality < 0 {
			return nil, ErrTruncated
		}
		decision := SymbolDecision{StartSeconds: position / input.SampleRate, OffsetHz: bestOffset, SoftEvidence: best.soft, Quality: best.quality}
		if best.soft > 0 {
			decision.Bit = 1
		}
		decisions = append(decisions, decision)
		position += period
		if math.Abs(best.soft) > .5 && best.quality > .25 {
			position += .25 * bestShift
			period = math.Max(sync.SamplesPerSymbol*.99, math.Min(sync.SamplesPerSymbol*1.01, period+.001*bestShift))
		}
		if bit == 63 && !bytes.Equal(assembleBytes(decisions), preamble[:]) {
			return nil, ErrNoPreamble
		}
		if bit == 95 {
			header := assembleBytes(decisions)
			length := uint64(binary.BigEndian.Uint32(header[8:12]))
			remainingBits := (length + 4) * 8
			if float64(remainingBits)*sync.SamplesPerSymbol*.98 > float64(len(input.I))-position+2 {
				return nil, fmt.Errorf("%w: advertised payload length %d", ErrTruncated, length)
			}
			totalBits += int(remainingBits)
		}
	}
	return decisions, nil
}

func assembleBytes(decisions []SymbolDecision) []byte {
	data := make([]byte, len(decisions)/8)
	for i, decision := range decisions {
		if i/8 < len(data) {
			data[i/8] = (data[i/8] << 1) | decision.Bit
		}
	}
	return data
}

// AssembleAndValidateFrame is the trust boundary between uncertain signal
// decisions and usable bytes. CRC detects accidental corruption, not tampering.
func AssembleAndValidateFrame(decisions []SymbolDecision, symbolSeconds float64) (VerifiedFrame, error) {
	if len(decisions) < 128 || len(decisions)%8 != 0 {
		return VerifiedFrame{}, ErrTruncated
	}
	for _, decision := range decisions {
		if decision.Bit > 1 {
			return VerifiedFrame{}, errors.New("invalid binary symbol")
		}
	}
	data := assembleBytes(decisions)
	if !bytes.Equal(data[:8], preamble[:]) {
		return VerifiedFrame{}, ErrNoPreamble
	}
	length := uint64(binary.BigEndian.Uint32(data[8:12]))
	if length+16 != uint64(len(data)) {
		return VerifiedFrame{}, ErrTruncated
	}
	end := len(data) - 4
	if crc32.ChecksumIEEE(data[12:end]) != binary.BigEndian.Uint32(data[end:]) {
		return VerifiedFrame{}, ErrChecksum
	}
	var confidence float64
	for _, decision := range decisions {
		confidence += math.Abs(decision.SoftEvidence)
	}
	return VerifiedFrame{Payload: data[12:end], StartSeconds: decisions[0].StartSeconds,
		EndSeconds: decisions[len(decisions)-1].StartSeconds + symbolSeconds, Confidence: confidence / float64(len(decisions))}, nil
}
