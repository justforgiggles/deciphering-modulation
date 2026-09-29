package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/justforgiggles/deciphering-modulation/am"
	"github.com/justforgiggles/deciphering-modulation/fm"
	"github.com/justforgiggles/deciphering-modulation/modulation"
	"github.com/justforgiggles/deciphering-modulation/pm_bpsk"
	"github.com/justforgiggles/deciphering-modulation/pm_qpsk"
	"github.com/justforgiggles/deciphering-modulation/qam"
)

func run(args []string) error {
	if len(args) > 0 && args[0] == "fdm" {
		return runFDM(args[1:])
	}
	if len(args) == 0 || args[0] != "encode" {
		return errors.New("usage: go run . fdm -in1 FIRST -in2 SECOND [options]; or go run . encode [-modulation am|fm|pm_bpsk|pm_qpsk|qam] [-in INPUT] [-out OUTPUT]")
	}
	flags := flag.NewFlagSet("encode", flag.ContinueOnError)
	in := flags.String("in", "data/image-small.png", "input file")
	out := flags.String("out", "modulated.wav", "new output file (must not exist)")
	kind := flags.String("modulation", "pm_bpsk", "modulation: am, fm, pm_bpsk, pm_qpsk, or qam")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *in == "" || *out == "" {
		return errors.New("specify nonempty input and output paths; no positional arguments are accepted")
	}
	processor, err := newProcessor(*kind, modulation.Config{CarrierHz: 1070, SampleRate: 48000, BitRate: 100})
	if err != nil {
		return err
	}
	return encodeFile(*in, *out, processor)
}

func newProcessor(kind string, config modulation.Config) (modulation.Processor, error) {
	switch kind {
	case "am":
		return am.New(config)
	case "fm":
		return fm.New(config)
	case "pm_bpsk":
		return pm_bpsk.New(config)
	case "pm_qpsk":
		return pm_qpsk.New(config)
	case "qam":
		return qam.New(config)
	default:
		return nil, fmt.Errorf("unsupported modulation %q: available: am, fm, pm_bpsk, pm_qpsk, qam", kind)
	}
}

func runFDM(args []string) error {
	flags := flag.NewFlagSet("fdm", flag.ContinueOnError)
	first := flags.String("in1", "", "first input file")
	second := flags.String("in2", "", "second input file")
	out := flags.String("out", "fdm.wav", "new output file (must not exist)")
	kind := flags.String("modulation", "am", "shared modulation: am, fm, pm_bpsk, pm_qpsk, or qam")
	carrier1 := flags.Float64("carrier1", 1070, "first carrier frequency in Hz (FM center)")
	carrier2 := flags.Float64("carrier2", 3070, "second carrier frequency in Hz (FM center)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *first == "" || *second == "" || *out == "" {
		return errors.New("specify -in1 FIRST -in2 SECOND and a nonempty output; no positional arguments are accepted")
	}
	p1, err := newProcessor(*kind, modulation.Config{CarrierHz: *carrier1, SampleRate: 48000, BitRate: 100})
	if err != nil {
		return err
	}
	p2, err := newProcessor(*kind, modulation.Config{CarrierHz: *carrier2, SampleRate: 48000, BitRate: 100})
	if err != nil {
		return err
	}
	return encodeFiles([]string{*first, *second}, *out, []modulation.Processor{p1, p2})
}
