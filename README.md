# Deciphering modulation

A small Go demonstration of bit-based amplitude (AM/OOK), frequency (FM/FSK),
phase (PM/BPSK and QPSK), and quadrature amplitude (16-QAM) modulation, using only the standard library. Requires Go 1.25 or newer.

After changing modulation code, carrier frequency, deviation, or bit rate, rebuild and generate
a fresh WAV with:

```sh
./run_am.sh
./run_fm.sh
./run_pm_bpsk.sh
./run_pm_qpsk.sh
./run_qam.sh
```

All five scripts use `data/image-small.png` and print the generated path under
`output/<am|fm|pm_bpsk|pm_qpsk|qam>-<timestamp>-<process-id>.wav`. Existing WAV files are preserved.

```sh
go run . encode -modulation am -in data/image-small.png -out modulated-bits.wav
go run . encode -modulation fm -in data/image-small.png -out modulated-fm.wav
go run . encode -modulation pm_bpsk -in data/image-small.png -out modulated-pm_bpsk.wav
go run . encode -modulation pm_qpsk -in data/image-small.png -out modulated-pm_qpsk.wav
go run . encode -modulation qam -in data/image-small.png -out modulated-qam.wav
afplay modulated-qam.wav  # macOS; start at low volume
go test ./...
```

To save a waveform PNG for a time range (in seconds), install Python's
`matplotlib` and `numpy`, then run:

```sh
python3 waveform.py path/to/audio.wav 0 0.03
python3 waveform.py path/to/audio.wav 0 0.03 -o waveform.png
python3 waveform.py path/to/audio.wav 0 0.03 --source data/image-small.png
```

The script accepts mono, 16-bit PCM WAV files and saves beside the input by
default. `--source` adds bit labels and boundaries using the original file bytes;
use it only when that file was encoded into the WAV. It reports an error if the
output file already exists.
Run `./run_waveform.sh` to create separate AM, FM, BPSK, QPSK, QAM, and FDM demo PNGs, plus plots
for WAVs already in `output/`. It uses the same 0–0.05 second window for each.
The labeled demo plots show the first five bits, `1 0 0 0 1`: AM drops to silence,
FM changes cycle spacing, and PM flips phase at the first bit boundary. Existing
PNGs are skipped. The QPSK demo labels pairs (`10`, `00`, `10`) at 20 ms symbol
boundaries, including the partially visible last symbol. The QAM demo labels `1000` and `1001` at 40 ms symbol boundaries. AM, FM,
and BPSK retain single-bit labels. Saved WAVs are plotted without labels because their source
files cannot reliably be inferred.

To label another QPSK waveform, provide its original source and the pair size:

```sh
python3 waveform.py path/to/pm_qpsk.wav 0 0.05 --source data/image-small.png --bits-per-symbol 2
```

`--bits-per-symbol` accepts 1 (default), 2 (QPSK), or 4 (16-QAM). Multi-bit
labels require `--source`.
Automatic output filenames use `-symbols.png` for multi-bit labels and `-bits.png`
for single-bit labels. A plot may show at most 32 labels.

`go run . encode` defaults to AM, `data/image-small.png`, and `modulated.wav`.
Output files must not already exist; choose a fresh output path for another run.
Failed conversions remove partial output. There is no decode command.

### Bytes → bits → carrier → WAV

Each input byte expands into eight bits, most-significant bit first, including
leading zeros. For example, `0x82` becomes `1 0 0 0 0 0 1 0`. PNG contents are read
as file bytes, without interpreting the pixels.

AM uses **on/off keying**: a zero bit is silence, and a one bit is a sine wave.
The carrier is **1,070 Hz**, the bit rate is **100 bits/second**, and the audio
sample rate is **48,000 Hz**. Each bit lasts 480 samples (10.7 carrier cycles), so
each file byte produces 3,840 samples.

```text
phase  = 2*pi*1070*sampleIndex/48000
sample = bit * sin(phase)
pcm    = round(sample * 32767)
```

The explicit nested loop in `am/am.go` holds each bit for 480 samples and advances
the carrier even during silence. The processor retains its carrier position
across calls. Returned samples are normalized to `[-1, 1]` and written as mono,
16-bit PCM WAV. Files are processed in 256-byte chunks.

### FM: continuous-phase binary FSK

FM uses frequency to carry each bit, keeping the carrier envelope at amplitude 1:

| Bit | Frequency | Duration | Carrier cycles |
|---|---:|---:|---:|
| 0 | 1,320 Hz | 10 ms | 13.2 |
| 1 | 820 Hz | 10 ms | 8.2 |

The center frequency is **1,070 Hz** with **±250 Hz deviation**. FM uses
100 bits/second and 48,000 samples/second, giving 480 samples per bit. FSK is the digital, two-frequency form of FM; this
is not an analog broadcast-FM encoder for speech or music.

The loop in `fm/fm.go` is:

```text
frequency = 1070 + (1 - 2*bit)*250
sample    = sin(phase)
phase     = (phase + 2*pi*frequency/48000) modulo (2*pi)
```

The phase accumulator advances continuously across bits and chunks, including
when the frequency changes. It avoids rounding the carrier period to an integer
number of samples. Bit timing still uses an integer samples-per-bit count: keep
`BitRate` a positive divisor of 48,000 for an exact rate. When changing FM tones,
keep both above zero and below the 24,000 Hz Nyquist frequency. AM and PM also use phase accumulators, so their carrier frequencies need not
divide the sample rate exactly.

### PM: binary phase-shift keying

PM uses a **1,070 Hz** carrier at **100 bits/second** and **48,000 samples/second**.
Each bit lasts 10 ms (480 samples, 10.7 carrier cycles). A zero selects a 0° phase
offset; a one selects a 180° offset, equivalent to inverting the sine wave:

```text
sample = (1 - 2*bit) * sin(carrierPhase)
carrierPhase = (carrierPhase + 2*pi*1070/48000) modulo (2*pi)
```

The underlying carrier phase continues across bits and chunks. The bit selects
an absolute phase offset: repeated ones stay inverted, rather than toggling phase
again. This is BPSK, not differential PSK. Bit changes intentionally produce
180° phase changes, while carrier frequency and envelope magnitude stay fixed.
Phase changes can occur away from zero crossings, creating abrupt waveform
changes. No pulse shaping is applied. Keep PM's bit rate a positive divisor of 48,000, and its carrier
above zero and below 24,000 Hz.

### QPSK: two bits per symbol

QPSK uses the same **1,070 Hz carrier**, **100 bits/second**, and **48,000 Hz**
audio rate as the other modules. Each symbol carries two bits, so the symbol
rate is **50 symbols/second**. A symbol lasts **20 ms (960 samples)**.

| Bit pair | Phase offset |
|---|---:|
| 00 | 45° |
| 01 | 135° |
| 11 | 225° |
| 10 | 315° |

This Gray mapping means neighboring phases differ by one bit. The first bit of
each pair is the more significant bit. The loop in `pm_qpsk/pm_qpsk.go` holds the chosen
offset for the full symbol:

```text
sample = sin(carrierPhase + symbolPhase)
carrierPhase = (carrierPhase + 2*pi*1070/48000) modulo (2*pi)
```

The underlying carrier keeps running across symbols and chunks. These are
absolute phase offsets, not differential encoding. The envelope has unit
magnitude; unlike 16-QAM, this constellation does not use multiple radii.

`Modulate` requires an even number of bits. It rejects incomplete pairs and
nonbinary values before changing state, without buffering or padding. File
chunks contain whole bytes, so they always contain complete symbols.
`SamplesPerBit()` returns 480 for WAV sizing, although each symbol produces 960
samples. At the same bit rate, QPSK has the same duration as the other modules;
it carries two bits in each symbol rather than doubling the configured bit rate.

### QAM: square 16-QAM

The `qam` module uses the same **1,070 Hz carrier**, **100 bits/second**, and
**48,000 Hz** audio as the other modules. Four bits form a symbol, so the symbol
rate is **25 symbols/second**: each symbol lasts **40 ms (1,920 samples)**.

The first two bits select the in-phase amplitude I; the last two select Q.
Both axes use this Gray mapping:

| Bit pair | Axis amplitude |
|---|---:|
| 00 | −3 |
| 01 | −1 |
| 11 | +1 |
| 10 | +3 |

```text
sample = (I*cos(phase) - Q*sin(phase)) / (3*sqrt(2))
phase  = (phase + 2*pi*1070/48000) modulo (2*pi)
```

I and Q remain fixed throughout each symbol while the carrier phase advances
across symbols and chunks. Normalizing by the largest constellation radius
keeps audio within `[-1, 1]`; a clamp handles floating-point rounding at the peaks.
Symbols retain their different amplitudes (radii 1/3, sqrt(5)/3, and 1 after
normalization), unlike QPSK's constant-radius constellation.

Input must contain complete groups of four binary values. Invalid input returns
an error before changing state; empty input is valid. Byte-based file chunks
always contain complete symbols. `SamplesPerBit()` remains 480, so the shared
WAV pipeline needs no special handling. The same bit rate means the same file
duration as the other modules, despite carrying four bits per symbol.

To label QAM symbols in a plot:

```sh
python3 waveform.py path/to/qam.wav 0 0.05 --source data/image-small.png --bits-per-symbol 4
```

### Chunk interface

```go
import (
    "github.com/justforgiggles/deciphering-modulation/am"
    "github.com/justforgiggles/deciphering-modulation/modulation"
)

// Inside a function returning error:
var processor modulation.Processor = &am.Processor{}
bits := modulation.BytesToBits([]byte{0x82})
samples, err := processor.Modulate(bits)
if err != nil {
    return err
}
// samples contains 8 * 480 = 3840 audio samples.
// Reuse processor for the next chunk.
```

The shared `modulation` package defines the contract:

```go
type Processor interface {
    Modulate(bits []byte) ([]float64, error)
    SampleRate() int
    SamplesPerBit() int
}
```

`Modulate` accepts unpacked bits (each slice element is `0` or `1`), returns a new
sample slice, and leaves the input unchanged. Empty chunks are allowed. Invalid
bits return an error without changing carrier state. QPSK also requires complete bit pairs, and QAM requires four-bit groups. Use a fresh processor
for each file; processors are not safe for concurrent use.

`main.go` only invokes the CLI and reports errors. `cli.go` selects the concrete
processor. `wav.go` depends on the shared interface and gets its timing from the
processor, so it imports none of the concrete modulation packages. To use another modulation, import its package and construct
`&fm.Processor{}`, `&pm_bpsk.Processor{}`, `&pm_qpsk.Processor{}`, or `&qam.Processor{}`. Each implementation owns its carrier state;
the interface is shared.

### Small demo image

`data/image-small.png` is a 128×128 copy of the original, prepared once with:

```sh
sips -Z 128 data/image.png --out data/image-small.png
```

The original image is preserved. The small PNG is **29,024 bytes**. At the current
settings it produces:

| Modulation | Bit rate | Duration | WAV size |
|---|---:|---:|---:|
| AM / FM / PM / QPSK / QAM | 100 bits/s | 2,321.92 s (38m 42s) | 222,904,364 bytes |

Resizing is not part of the Go application. WAV size is
`44 + inputBytes * 8 * samplesPerBit * 2`. Standard RIFF limits allow at most
559,240 input bytes for all five modules at these settings. Oversized
files are rejected before creating output. The original full-size PNG exceeds
these limits.

This version focuses on modulation. It has no decoder, framing, checksum, or
speaker/microphone transport support. AM changes amplitude, FM changes frequency,
PM changes phase at bit boundaries, and QPSK changes phase at symbol boundaries. QAM changes amplitude and phase at symbol boundaries. None applies pulse shaping or
bandwidth-limiting filters. Existing generated audio is not updated automatically.

### Current demonstration settings

All five modules currently use a 1,070 Hz carrier (FM center), a 100-bit/s data
rate, and 48 kHz mono, 16-bit PCM audio. FM deviates by ±250 Hz. Their constants
remain independent, so each can be adjusted separately. Keep bit rates positive
and evenly divisible into 48,000 for exact integer sample timing, and keep all
carrier/tone frequencies above zero and below 24,000 Hz.

These are educational settings, not complete modem standards or a 56k handshake.
The original unit peak amplitude is preserved. Run the scripts again after
changing settings; existing WAV files are not overwritten.

### FDM: two files on separate carriers

```sh
./run_fdm.sh first.bin second.bin                 # AM, 1070 Hz and 3070 Hz
./run_fdm.sh first.bin second.bin qam 1500 4500
go run . fdm -in1 first.bin -in2 second.bin -modulation fm -carrier1 1070 -carrier2 3070 -out combined.wav
```

Each file becomes MSB-first bits and passes through its own processor, with
independent carrier phase. Both channels use the selected modulation (`am`,
`fm`, `pm_bpsk`, `pm_qpsk`, or `qam`) and their configured bit rate. Carrier flags
select FM center frequencies; deviation remains ±250 Hz. Defaults are AM,
1,070/3,070 Hz, and `fdm.wav`. The script rebuilds and creates a unique
`output/fdm-<timestamp>-<process-id>.wav`.

The `fdm.Mix(first, second []float64) ([]float64, error)` function combines
normalized audio arrays as `(first + second) / 2`, leaving both inputs unchanged.
Missing samples are silence. Output lasts as long as the longer file; the
remaining channel stays at half gain when the shorter file ends. Empty files
are supported, including a header-only WAV when both are empty. Streaming uses
the existing 256-byte chunks and rejects mismatched processor timing.

All five modulation packages expose `New(carrierHz float64) (*Processor, error)`;
their zero-value processors retain the original defaults. Only nonfinite
frequencies are rejected. Identical, overlapping, zero, negative, and
above-Nyquist carriers are permitted for experiments. There is no automatic
spacing, guard band, filtering, or demultiplexer. Distinct carriers with adequate
separation provide FDM; overlapping choices simply produce a mixed waveform.

`./run_waveform.sh` also generates an unlabeled FDM demo from two independent
one-byte inputs, `0x89` and `0x76`, and plots saved FDM WAVs. A composite waveform
does not have a single source-bit sequence to label.
