package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/justforgiggles/deciphering-modulation/demodulation"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is the workshop roadmap. Each named call below owns one processing stage.
func run(ctx context.Context, args []string, diagnostics io.Writer) error {
	config := demodulation.DefaultConfig()
	flags := flag.NewFlagSet("demodulate", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	input := flags.String("input", "", "input WAV, MP4 or MP3 file")
	output := flags.String("output-dir", "", "new directory for verified payloads")
	flags.Float64Var(&config.CarrierHz, "carrier", config.CarrierHz, "nominal carrier frequency in Hz")
	flags.Float64Var(&config.BandwidthHz, "bandwidth", config.BandwidthHz, "chirp sweep width in Hz")
	flags.Float64Var(&config.BitRate, "bit-rate", config.BitRate, "nominal bits per second")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *input == "" || *output == "" || flags.NArg() != 0 {
		return errors.New("usage: demodulate -input media -output-dir new-directory")
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(*output); err == nil {
		return fmt.Errorf("output directory already exists: %s", *output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	pcmPath, cleanup, err := demodulation.ConvertAudioToPCM(ctx, *input)
	if err != nil {
		return err
	}
	defer cleanup()
	audio, err := demodulation.ReadAudioSamples(pcmPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(diagnostics, "Audio: %d samples, %.3f seconds, %.0f Hz\n", len(audio.Samples), float64(len(audio.Samples))/audio.SampleRate, audio.SampleRate)
	centered := demodulation.RemoveDCOffset(audio)
	carrier := demodulation.GenerateQuadratureCarrier(len(centered.Samples), centered.SampleRate, config.CarrierHz)
	mixed := demodulation.MixToBaseband(centered, carrier)
	factor := demodulation.DownsampleFactor(mixed.SampleRate, config)
	filter := demodulation.DesignLowPassFilter(mixed.SampleRate, factor)
	filtered := demodulation.ApplyFIRFilter(mixed, filter)
	baseband := demodulation.Downsample(filtered, factor)

	candidates, err := demodulation.FindPreambleCandidates(ctx, baseband, config)
	if err != nil {
		return err
	}
	fmt.Fprintf(diagnostics, "Preamble candidates: %d; baseband %.0f samples/s\n", len(candidates), baseband.SampleRate)
	var frames []demodulation.VerifiedFrame
	for _, candidate := range candidates {
		if len(frames) > 0 && candidate.StartSample/baseband.SampleRate < frames[len(frames)-1].EndSeconds {
			continue
		}
		sync := demodulation.RefineSynchronization(baseband, config, candidate)
		decisions, err := demodulation.DetectSymbolsWithTracking(ctx, baseband, config, sync)
		var frame demodulation.VerifiedFrame
		if err == nil {
			frame, err = demodulation.AssembleAndValidateFrame(decisions, sync.SamplesPerSymbol/baseband.SampleRate)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fmt.Fprintf(diagnostics, "Rejected candidate at %.6fs: %v\n", sync.StartSample/baseband.SampleRate, err)
			continue
		}
		frames = append(frames, frame)
		fmt.Fprintf(diagnostics, "Frame %d: %.6f–%.6fs, %d bytes, CRC32 verified, confidence %.3f\n", len(frames), frame.StartSeconds, frame.EndSeconds, len(frame.Payload), frame.Confidence)
	}
	if len(frames) == 0 {
		return errors.New("no CRC-verified CSS frames recovered")
	}
	return demodulation.WriteVerifiedPayloads(ctx, *output, frames)
}
