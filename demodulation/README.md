# Binary CSS demodulation, one stage at a time

This is an offline Go receiver for this repository's **binary chirp spread
spectrum** format. Start the workshop in `cmd/demodulate/main.go`: its `run`
function reads like the signal-processing pipeline. Follow each named call into
this package. There are no Go DSP dependencies, FFT shortcuts, or hidden payload
lookups. FFmpeg is the external dependency for compressed-media decoding.

The implementation favors visible sample arrays and ordinary loops over speed.
In particular, the mixer, FIR convolution, reference chirps, and dechirped output
are explicit arrays. Intermediate CSV/plot exports are deliberately omitted.

## Run the supplied recording

From the repository root, with Go 1.25 and FFmpeg on `PATH`:

```sh
go run ./cmd/demodulate \
  -input 'data/WhatsApp Video 2026-10-11 at 18.49.42.mp4' \
  -output-dir output/mp4-decoded

cmp data/lorem-ipsum-1.txt output/mp4-decoded/frame-001.bin
cmp data/lorem-ipsum-1.txt output/mp4-decoded/frame-002.bin
```

The output directory must be new; its parent must already exist. The command
never overwrites existing results. It writes one `frame-NNN.bin` for each
CRC-verified transmission, preserving repeated payloads as separate files.
Overlapping detections within an already verified transmission are skipped.

All diagnostics go to stderr. A rejected candidate is reported, then reception
continues. No verified frames, invalid input, conversion failure, cancellation,
or output failure returns a nonzero exit code. If some candidates fail but
others verify, the verified frames are saved and the command succeeds with the
rejections visible in its diagnostics. A failed write removes the newly created
output directory. Interrupting with Ctrl-C cancels conversion and receiver search.

The defaults are `-carrier 6000 -bandwidth 500 -bit-rate 100`. Older 8 kHz
recordings need `-carrier 8000`. These are known transmitter parameters, not
values inferred from arbitrary audio. Supported configuration bounds are
20–500 bit/s and 100–2000 Hz bandwidth, with the carrier and its search band
strictly below the 24 kHz Nyquist frequency. These bounds prevent invalid DSP
configurations; they are not a guarantee of decoding every channel in that range.

WAV and MP3 inputs use the same command. To create a new clean source, run
`go run . css` and supply the generated filename to `-input`.

## The signal path

### 1. Convert media into samples

`ConvertAudioToPCM` decodes MP4/MP3 and incompatible WAV formats into a private
temporary WAV: mono, 48,000 samples/s, signed 16-bit PCM. It passes an argument
list directly to FFmpeg, without a shell. The first audio stream is selected;
video is ignored. Stereo is downmixed and other sample rates are resampled.
Those format conversions are necessary here, but there is no normalization,
speech enhancement, silence removal, or time stretching. Stereo signals with
opposite phase can cancel when downmixed; this supplied recording passes the
two-frame acceptance test with that conversion.

Compatible PCM WAVs bypass FFmpeg. Temporary conversion files are removed on
failure and by the caller's deferred cleanup on success. Conversion is capped
at 301 seconds and the reader rejects anything exceeding 300 seconds: the extra
second prevents an oversized recording from being silently accepted as a valid
truncated capture. FFmpeg errors retain bounded diagnostic output. Local
file/pipe protocols are allowed; network protocols are not.

`ReadAudioSamples` walks RIFF chunks rather than assuming every WAV has a
44-byte header. It checks declared sizes, padding, format, byte rate, alignment,
duplicate chunks, and input limits. Each signed integer is divided by 32768,
yielding a floating-point sample in [-1, 1). WAV input is bounded to 64 MiB and
decoded audio to 300 seconds. The supplied video is about 24.68 seconds after
decoding; `data/audio.wav` is an unrelated older file.

### 2. Remove DC offset

`RemoveDCOffset` adds all samples, divides by their count to find the mean, and
subtracts that mean from each sample into a new array. The result is centered
around zero. This removes a constant recording bias without changing the
signal's relative volume or filling silent portions with guessed information.

### 3. Build and apply the local oscillator

`GenerateQuadratureCarrier` creates two full arrays: cosine and negative sine
at the nominal carrier. `MixToBaseband` then multiplies each audio sample by
the corresponding element of each oscillator array.

Multiplication creates sum and difference frequencies. The desired chirps move
from approximately 5,750–6,250 Hz to approximately -250–250 Hz. An unwanted
image appears near twice the carrier frequency and must be removed.

The two branches are named **I** and **Q**. A single cosine mixer could lose the
signal at an unfavorable carrier phase. Two branches preserve both quadratures,
so the subsequent energy measurement can receive any fixed phase.

### 4. Low-pass filter, then downsample

`DownsampleFactor` selects a reduced sample rate with sufficient room for both
chirp bandwidth and synchronization search, retaining at least 12 samples/bit.
For the defaults, its factor is 16: 48 kHz becomes 3 kHz, or 30 samples per bit.

`DesignLowPassFilter` explicitly builds a finite impulse response:

1. Calculate the ideal low-pass sinc samples.
2. Multiply them by a Hamming window to taper the finite ends.
3. Divide the coefficients by their sum so constant input retains its gain.

At the defaults there are 513 taps with a 1,200 Hz cutoff. This retains the
±250 Hz sweep and ±200 Hz frequency-search margin while suppressing the mixer
image and frequencies that would alias after decimation. The finite transition
band is why the cutoff is below the new 1,500 Hz Nyquist frequency.

`ApplyFIRFilter` slides these coefficients over the I and Q arrays and sums
the weighted samples. It computes every output sample for clarity. Because
this is offline processing, the symmetric filter is centered and zero-padded
at the boundaries. Its half-length delay is compensated, preserving timestamps.
`Downsample` then copies every sixteenth filtered sample. Reversing these two
operations would fold unwanted frequencies into the wanted band.

### 5. Locate the known preamble

`FindPreambleCandidates` evaluates candidate starting positions throughout the
recording. It searches carrier errors from -200 to +200 Hz in 25 Hz steps and
symbol periods within ±1% in 0.1% steps. The start grid is approximately one
twelfth of a symbol. Window scores are reused across timing hypotheses.

At each candidate symbol, compare its up-chirp and down-chirp evidence. The
signed score `(downEnergy-upEnergy)/(downEnergy+upEnergy)` is negative for 0
and positive for 1. Multiply those scores by the expected preamble signs and
average the 64 results. Candidates need a mean score of at least 0.65. Nearby
hypotheses are merged by start position; no payload content participates.

`RefineSynchronization` performs an understandable coordinate search: try
earlier/later start, shorter/longer period, and lower/higher frequency; retain
improvements and reduce the step sizes. `InterpolateSymbol` makes fractional
sample windows using linear interpolation rather than repeatedly rounding time.

### 6. Generate references, dechirp, and detect each symbol

`GenerateReferenceChirp` creates the two reference arrays by accumulating phase
from each sample interval's midpoint frequency. For a zero bit the sweep moves
up; for a one bit it moves down. This mirrors the transmitter's construction.

For each reference, `Dechirp` multiplies the received samples by its conjugate:

```text
dechirped I = received I × reference I + received Q × reference Q
dechirped Q = received Q × reference I − received I × reference Q
```

A matching sweep becomes a nearly stationary phasor. A mismatched sweep keeps
rotating. `MeasureCoherentEnergy` sums each dechirped branch separately, squares
the two sums, and adds them. Matching samples reinforce; rotating samples tend
to cancel. This dechirp-and-sum operation is a matched filter. No FFT is needed
for the binary up/down hypotheses used here.

`DetectSymbolsWithTracking` performs this comparison using a reference bank of
residual offsets from -200 to +200 Hz in 5 Hz steps **for each symbol**. It also
tests 13 nearby start positions spanning ±5% of a symbol. It selects the pair
of timing/frequency hypotheses with the highest normalized matched energy.
The selected up/down soft-score sign determines the bit; its magnitude is
reported as evidence, not as a calibrated probability.

Strong, coherent symbols adjust the next start by a quarter of the timing
correction and make a much smaller adjustment to the symbol period. Period
adjustments are bounded to ±1% of the acquired estimate. Weak symbols cannot
steer this timing loop. This is deliberate bounded tracking, not an assumption
that all subsequent symbols have exactly the first symbol's timing/frequency.

The supplied second transmission illustrates why tracking matters: a fixed
preamble frequency hypothesis yielded a valid header but corrupted the later
payload. Searching residual frequency during reception recovered its valid CRC.
Estimated frequency and timing can trade off on a chirp; these estimates are
receiver hypotheses, not precise independent measurements of physical Doppler.

### 7. Reassemble bytes and verify the frame

The detector first reads only 96 bits: preamble and length. It checks the
preamble exactly and bounds the advertised length against remaining audio
before continuing. `AssembleAndValidateFrame` assembles bytes MSB-first and
validates the complete structure:

| Field | Size | Meaning |
| --- | --- | --- |
| Preamble | 8 bytes | `a2 3c ee fc a7 84 df 4b` |
| Payload length | 4 bytes | Unsigned big-endian byte count |
| Payload | Declared length | Original bytes, including zeros/newlines |
| CRC | 4 bytes | Big-endian IEEE CRC32 of the payload |

Only CRC-verified payloads reach `WriteVerifiedPayloads`. CRC detects accidental
corruption; it neither authenticates the sender nor repairs lost symbols.
The known text file and generated source WAV are used only by tests, never by
the decoder to pick bits, fill gaps, or select a favorable checksum.

## Verification and measured results

```sh
GOCACHE=/tmp/css-go-cache go test ./...

# Required real-recording acceptance: explicitly enable the external fixture.
CSS_RECORDING="$PWD/data/WhatsApp Video 2026-10-11 at 18.49.42.mp4" \
  GOCACHE=/tmp/css-go-cache go test ./cmd/demodulate -run '^TestRecordedMP4$' -count=1 -v

GOCACHE=/tmp/css-go-cache go vet ./demodulation ./cmd/demodulate
```

The MP4 acceptance test requires exactly two output files and compares each
independently with `data/lorem-ipsum-1.txt`. Without `CSS_RECORDING`, it explicitly
skips. If the variable is set but the fixture or FFmpeg is missing, it fails.
The regular MP3 test explicitly skips when FFmpeg is unavailable.

Measured on the supplied recording:

| Run | Start | End | Verified bytes | Mean absolute evidence |
| --- | --- | --- | --- | --- |
| 1 | 10.875827 s | 16.714993 s | 57 | 0.806 |
| 2 | 17.574667 s | 23.414912 s | 57 | 0.643 |

The payload from both is `Lorem ipsum dolor sit amet, consectetur adipiscing elit.`
followed by a newline. Lower evidence on the second frame is acceptable because
its independently calculated CRC verifies.

Unit tests cover array operations, phase-independent detection, FIR passband and
stopband, timestamp preservation, interpolation, frame bounds, malformed WAV,
input limits, cancellation, and output protection. Synthetic transmissions cover
binary/empty payloads, DC bias, fractional timing, noise, a modest echo, clock
mismatch, frequency offset/drift, dropped audio, corruption, truncation, and
recovery after a rejected frame. MP3 conversion is tested through actual FFmpeg.

## Library use and limits

Use the named exported stages in the same order as the command. `Audio` carries
real samples and sample rate; `IQ` carries equal-length I/Q arrays and their rate;
`Synchronization` expresses start/period in baseband samples; `SymbolDecision`
records bit evidence; `VerifiedFrame` carries recovered bytes and timestamps.
Numeric stages expect finite, aligned arrays and valid rates/configuration;
the media reader and `Config.Validate` establish those conditions for the CLI.
For custom in-memory signals, establish the same conditions before calling them.

There is no streaming input, transmitter discovery, speech enhancement,
error-correcting code, or adaptive multipath equalizer. This is binary CSS,
not a LoRa decoder. Sample arrays and explicit convolution intentionally use
more memory and CPU than an optimized receiver. The valid-frame output rules
and error handling are complete for this protocol; the supplied recording and
synthetic tests establish the tested channel conditions, not universal acoustic
reliability. No microphone or transmitter configuration is changed by this tool.
