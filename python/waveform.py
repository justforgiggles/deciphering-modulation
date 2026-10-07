"""Plot 1.45–1.55 seconds of the filtered mixed WAV waveform."""

from pathlib import Path
from math import cos, pi, sin

import matplotlib
matplotlib.use("Agg")

import matplotlib.pyplot as plt
import numpy as np

from audio_io import read_pcm


ROOT = Path(__file__).resolve().parent.parent
INPUT_FILE = ROOT / "output/mixed.wav"
OUTPUT_FILE = ROOT / "output/waveform_mixed_1.45-1.60.png"
START_SECONDS = 1.45
END_SECONDS = 1.6

CARRIER_HZ = 1070
BANDWIDTH_HZ = 500
BASEBAND_CUTOFF_HZ = 250
BIT_RATE = 100

def lowpass_filter(x, cutoff_hz, sample_rate, num_taps=101):
    # Normalized cutoff frequency: 0 < fc < 0.5
    fc = cutoff_hz / sample_rate

    if not 0 < fc < 0.5:
        raise ValueError("cutoff_hz must be between 0 and sample_rate / 2")

    # Symmetric sample positions
    n = np.arange(num_taps) - (num_taps - 1) / 2

    # Ideal low-pass impulse response
    h = 2 * fc * np.sinc(2 * fc * n)

    # Apply a window to reduce ringing
    h *= np.hamming(num_taps)

    # Normalize so DC gain is 1
    h /= np.sum(h)

    # Apply filter
    return np.convolve(x, h, mode="same")

def main():
    samples, sample_rate = read_pcm(INPUT_FILE)
    start = round(START_SECONDS * sample_rate)
    end = round(END_SECONDS * sample_rate)
    if end > len(samples):
        raise ValueError(f"{INPUT_FILE} is shorter than {END_SECONDS} seconds")
    
    ### <CODE>
    
    result = lowpass_filter(samples, 500, sample_rate, 101)
    
    ### </CODE>

    times = []
    amplitudes = []
    for index in range(start, end):
        times.append(index / sample_rate)
        amplitudes.append(samples[index] / 32768)

    fig, ax = plt.subplots(figsize=(12, 4))
    ax.plot(times, amplitudes, linewidth=0.5)
    ax.set(xlabel="Time (s)", ylabel="Amplitude",
           xlim=(START_SECONDS, END_SECONDS))
    fig.tight_layout()
    fig.savefig(OUTPUT_FILE, dpi=150)
    plt.close(fig)
    print(f"Wrote {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
