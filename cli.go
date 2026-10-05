package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justforgiggles/deciphering-modulation/am"
	"github.com/justforgiggles/deciphering-modulation/css"
	"github.com/justforgiggles/deciphering-modulation/css_16"
	"github.com/justforgiggles/deciphering-modulation/fm"
	"github.com/justforgiggles/deciphering-modulation/modulation"
	"github.com/justforgiggles/deciphering-modulation/pm_bpsk"
	"github.com/justforgiggles/deciphering-modulation/pm_qpsk"
	"github.com/justforgiggles/deciphering-modulation/qam"
)

func run(args []string) error {
	const usage = "usage: go run . am|fm|pm_bpsk|pm_qpsk|qam|css|css_16|fdm"
	if len(args) != 1 {
		return errors.New(usage)
	}
	kind := args[0]
	carriers := []float64{1070}
	if kind == "fdm" {
		kind = "am"
		carriers = append(carriers, 3070)
	}
	var inputs []string
	var processors []modulation.Processor
	for i, carrier := range carriers {
		processor, err := newProcessor(kind, modulation.Config{CarrierHz: carrier, SampleRate: 48000, BitRate: 100})
		if err != nil {
			return fmt.Errorf("%w; %s", err, usage)
		}
		inputs = append(inputs, fmt.Sprintf("data/lorem-ipsum-%d.txt", i+1))
		processors = append(processors, processor)
	}
	output, err := filepath.Abs(filepath.Join("output", fmt.Sprintf("%s-%s-%d.wav", args[0], time.Now().Format("20060102-150405.000000000"), os.Getpid())))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if err := encodeFiles(inputs, output, processors); err != nil {
		return err
	}
	fmt.Printf("Generated: %s\n", output)
	return nil
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
	case "css":
		return css.New(config)
	case "css_16":
		return css_16.New(config)
	default:
		return nil, fmt.Errorf("unsupported modulation %q: available: am, fm, pm_bpsk, pm_qpsk, qam, css, css_16", kind)
	}
}
