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
	processor, err := newProcessor(*kind, nil)
	if err != nil {
		return err
	}
	return encodeFile(*in, *out, processor)
}

// A nil carrier preserves the selected module's zero-value defaults.
func newProcessor(kind string, carrier *float64) (modulation.Processor, error) {
	switch kind {
	case "am":
		if carrier != nil {
			return am.New(*carrier)
		}
		return &am.Processor{}, nil
	case "fm":
		if carrier != nil {
			return fm.New(*carrier)
		}
		return &fm.Processor{}, nil
	case "pm_bpsk":
		if carrier != nil {
			return pm_bpsk.New(*carrier)
		}
		return &pm_bpsk.Processor{}, nil
	case "pm_qpsk":
		if carrier != nil {
			return pm_qpsk.New(*carrier)
		}
		return &pm_qpsk.Processor{}, nil
	case "qam":
		if carrier != nil {
			return qam.New(*carrier)
		}
		return &qam.Processor{}, nil
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
	p1, err := newProcessor(*kind, carrier1)
	if err != nil {
		return err
	}
	p2, err := newProcessor(*kind, carrier2)
	if err != nil {
		return err
	}
	return encodeFiles([]string{*first, *second}, *out, []modulation.Processor{p1, p2})
}
