from pathlib import Path

import numpy as np

from audio_io import read_pcm, write_pcm


ROOT = Path(__file__).resolve().parent.parent
NOISE_FILE = ROOT / "data/audio.wav"
SIGNAL_FILE = ROOT / "output/am-20261005-112510.032570000-17715.wav"
OUTPUT_FILE = ROOT / "output/mixed.wav"
OFFSET_SECONDS = 1.5
SNR_DB = 0


def process(signal, noise, sample_rate):
    offset = round(OFFSET_SECONDS * sample_rate)
    print(f"offset: {offset}")
    if offset < 0 or offset >= len(noise):
        raise ValueError("offset must fall within the noise audio")

    end = min(len(noise), offset + len(signal))
    print(f"end: {end}")

    signal_part = signal[: end - offset].astype(np.float64)
    noise_part = noise[offset:end].astype(np.float64)

    signal_rms = np.sqrt(np.mean(signal_part**2))
    print(f"signal_rms: {signal_rms}")

    noise_rms = np.sqrt(np.mean(noise_part**2))
    print(f"noise_rms: {noise_rms}")

    if signal_rms == 0 or noise_rms == 0:
        raise ValueError("both files must have non-silent audio during the overlap")

    mixed = noise[:end].astype(np.float64)
    mixed[offset:] += signal_part * (noise_rms / signal_rms) * 10 ** (SNR_DB / 20)

    peak = np.max(np.abs(mixed))
    print(f"peak: {peak}")

    if peak > 32767:
        mixed *= 32767 / peak

    return np.rint(mixed).astype("<i2")


def main():
    signal, signal_rate = read_pcm(SIGNAL_FILE)
    noise, noise_rate = read_pcm(NOISE_FILE)
    if signal_rate != noise_rate:
        raise ValueError("signal and noise sample rates must match")

    result = process(signal, noise, signal_rate)
    write_pcm(OUTPUT_FILE, result, signal_rate)
    print(f"Wrote {OUTPUT_FILE} ({len(result) / signal_rate:.2f} seconds)")


if __name__ == "__main__":
    main()
