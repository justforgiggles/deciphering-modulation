"""Save a waveform PNG for a time range in a mono, 16-bit PCM WAV file."""

import argparse
import math
from pathlib import Path
import wave

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np


def save_waveform(
    wav_path: Path, start: float, end: float, png_path: Path, source: bytes | None = None,
    bits_per_symbol: int = 1,
) -> None:
    if bits_per_symbol not in (1, 2, 4):
        raise ValueError("bits per symbol must be 1, 2, or 4")
    if bits_per_symbol > 1 and source is None:
        raise ValueError("multi-bit symbol labels require source bytes")
    with wave.open(str(wav_path), "rb") as wav:
        if wav.getnchannels() != 1 or wav.getsampwidth() != 2 or wav.getcomptype() != "NONE":
            raise ValueError("input must be a mono, 16-bit PCM WAV file")

        rate = wav.getframerate()
        duration = wav.getnframes() / rate
        if not (math.isfinite(start) and math.isfinite(end) and 0 <= start < end <= duration):
            raise ValueError(f"times must satisfy 0 <= START < END <= {duration:g} seconds")

        first = int(start * rate)
        last = min(wav.getnframes(), math.ceil(end * rate))
        wav.setpos(first)
        count = last - first
        if source is not None:
            bit_count = len(source) * 8
            if not bit_count or wav.getnframes() % bit_count:
                raise ValueError("WAV length does not match the source byte count")
            frames_per_bit = wav.getnframes() // bit_count
            if not frames_per_bit:
                raise ValueError("WAV has no samples per source bit")
            frames_per_symbol = frames_per_bit * bits_per_symbol
            if (last - 1) // frames_per_symbol - first // frames_per_symbol >= 32:
                raise ValueError("select a shorter interval to show symbol labels")

        fig, ax = plt.subplots(figsize=(12, 4))
        try:
            if count <= 10_000:
                samples = np.frombuffer(wav.readframes(count), dtype="<i2") / 32768
                ax.plot((first + np.arange(len(samples))) / rate, samples, linewidth=0.7)
            else:
                step = math.ceil(count / 2_000)
                times, lows, highs = [], [], []
                for offset in range(0, count, step):
                    size = min(step, count - offset)
                    samples = np.frombuffer(wav.readframes(size), dtype="<i2")
                    times.append((first + offset + size / 2) / rate)
                    lows.append(samples.min() / 32768)
                    highs.append(samples.max() / 32768)
                ax.vlines(times, lows, highs, linewidth=0.7)

            if source is not None:
                for symbol_index in range(first // frames_per_symbol, (last - 1) // frames_per_symbol + 1):
                    symbol_start = symbol_index * frames_per_symbol / rate
                    symbol_end = (symbol_index + 1) * frames_per_symbol / rate
                    left, right = max(start, symbol_start), min(end, symbol_end)
                    if left >= right:
                        continue
                    if symbol_start > start:
                        ax.axvline(symbol_start, color="gray", linestyle="--", linewidth=0.8)
                    first_bit = symbol_index * bits_per_symbol
                    label = "".join(str((source[i // 8] >> (7 - i % 8)) & 1)
                                    for i in range(first_bit, first_bit + bits_per_symbol))
                    ax.text((left + right) / 2, 1.13, label, ha="center", va="center",
                            fontweight="bold")
            ax.set(xlim=(start, end), ylim=(-1.05, 1.25) if source is not None else (-1, 1),
                   xlabel="Time (seconds)", ylabel="Amplitude", title=wav_path.stem)
            fig.tight_layout()
            with png_path.open("xb") as output:
                try:
                    fig.savefig(output, format="png")
                except Exception:
                    png_path.unlink()
                    raise
        finally:
            plt.close(fig)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("wav", type=Path, help="mono 16-bit PCM WAV file")
    parser.add_argument("start", type=float, help="start time in seconds")
    parser.add_argument("end", type=float, help="end time in seconds")
    parser.add_argument("-o", "--output", type=Path, help="output PNG path")
    parser.add_argument("--source", type=Path, help="original bytes encoded into the WAV; add bit labels")
    parser.add_argument("--bits-per-symbol", type=int, choices=(1, 2, 4), default=1,
                        help="group source bits into symbols (2 for QPSK, 4 for 16-QAM)")
    args = parser.parse_args()
    suffix = ("-symbols" if args.bits_per_symbol > 1 else "-bits") if args.source else ""
    output = args.output or args.wav.with_name(
        f"{args.wav.stem}-waveform-{args.start:g}-{args.end:g}"
        f"{suffix}.png"
    )
    try:
        save_waveform(args.wav, args.start, args.end, output,
                      args.source.read_bytes() if args.source else None, args.bits_per_symbol)
    except (OSError, ValueError, wave.Error) as exc:
        parser.error(str(exc))
    print(output)


if __name__ == "__main__":
    main()
