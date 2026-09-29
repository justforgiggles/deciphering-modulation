package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/justforgiggles/deciphering-modulation/am"
	"github.com/justforgiggles/deciphering-modulation/fm"
	"github.com/justforgiggles/deciphering-modulation/pm_bpsk"
	"github.com/justforgiggles/deciphering-modulation/pm_qpsk"
	"github.com/justforgiggles/deciphering-modulation/qam"
)

func run(args []string) error {
	if len(args) == 0 || args[0] != "encode" {
		return errors.New("usage: go run . encode [-modulation am|fm|pm_bpsk|pm_qpsk|qam] [-in INPUT] [-out OUTPUT]")
	}
	flags := flag.NewFlagSet("encode", flag.ContinueOnError)
	in := flags.String("in", "data/image-small.png", "input file")
	out := flags.String("out", "modulated.wav", "new output file (must not exist)")
	kind := flags.String("modulation", "am", "modulation: am, fm, pm_bpsk, pm_qpsk, or qam")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *in == "" || *out == "" {
		return errors.New("specify nonempty input and output paths; no positional arguments are accepted")
	}
	// Only this selection point knows which concrete modulation to construct.
	switch *kind {
	case "am":
		return encodeFile(*in, *out, &am.Processor{})
	case "fm":
		return encodeFile(*in, *out, &fm.Processor{})
	case "pm_bpsk":
		return encodeFile(*in, *out, &pm_bpsk.Processor{})
	case "pm_qpsk":
		return encodeFile(*in, *out, &pm_qpsk.Processor{})
	case "qam":
		return encodeFile(*in, *out, &qam.Processor{})
	default:
		return fmt.Errorf("unsupported modulation %q: available: am, fm, pm_bpsk, pm_qpsk, qam", *kind)
	}
}
