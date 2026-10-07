"""Plot 1.45–1.55 seconds of the filtered mixed WAV waveform."""

from pathlib import Path
from math import cos, pi, sin

import matplotlib
matplotlib.use("Agg")

import matplotlib.pyplot as plt
import numpy as np
from scipy.signal import butter, sosfiltfilt

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

def bandpass_filter(x, lowcut, highcut, fs, order=4):
    sos = butter(order, [lowcut, highcut], btype="bandpass", fs=fs, output="sos")
    return sosfiltfilt(sos, x)

def low_pass(samples, alpha):
    result = np.empty(len(samples), dtype=np.float64)
    previous = 0.0
    for index, sample in enumerate(samples):
        previous += alpha * (sample - previous)
        result[index] = previous
    return result

def downconvert(samples, sample_rate):
    alpha = 1 / (1 + sample_rate / (2 * pi * BASEBAND_CUTOFF_HZ))
    in_phase = np.empty(len(samples), dtype=np.float64)
    quadrature = np.empty(len(samples), dtype=np.float64)
    for index, sample in enumerate(samples):
        phase = 2 * pi * CARRIER_HZ * index / sample_rate
        amplitude = 2 * float(sample) / 32768
        in_phase[index] = amplitude * cos(phase)
        quadrature[index] = -amplitude * sin(phase)

    in_phase = low_pass(low_pass(in_phase, alpha), alpha)
    quadrature = low_pass(low_pass(quadrature, alpha), alpha)
    return in_phase, quadrature

def matched_filter(samples, sample_rate):
    window = round(sample_rate / BIT_RATE)
    if window < 1:
        raise ValueError("sample rate must provide at least one sample per bit")

    in_phase, quadrature = downconvert(samples, sample_rate)
    response = np.empty(len(samples), dtype=np.float64)
    sum_i = sum_q = 0.0
    for index in range(len(samples)):
        sum_i += in_phase[index]
        sum_q += quadrature[index]
        if index >= window:
            sum_i -= in_phase[index - window]
            sum_q -= quadrature[index - window]
        response[index] = (sum_i / window) ** 2 + (sum_q / window) ** 2
    return response


def main():
    samples, sample_rate = read_pcm(INPUT_FILE)
    start = round(START_SECONDS * sample_rate)
    end = round(END_SECONDS * sample_rate)
    if end > len(samples):
        raise ValueError(f"{INPUT_FILE} is shorter than {END_SECONDS} seconds")
    
    ### <CODE>

    # f = 1000
    # t = np.arange(len(samples)) / sample_rate
    # phase = np.pi / 1
    # sine = np.sin(2 * np.pi * f * t + phase)
    # cosine = np.cos(2 * np.pi * f * t + phase)

    # samples = samples * sine
    
    # samples = bandpass_filter(samples, 1000, 1140, sample_rate)
    samples = matched_filter(samples, sample_rate)
    
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
