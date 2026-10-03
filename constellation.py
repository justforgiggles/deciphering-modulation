"""Recover I/Q from demo WAVs using known carrier and symbol synchronization.

Usage: python3 constellation.py am output/am-....wav
       python3 constellation.py --self-test
Requires NumPy and Matplotlib. Source image bytes are never used for recovery.
"""
import argparse
from pathlib import Path
import wave

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np

RATE = 48000
CARRIER = 1070
SPB = 480
SIZES = {"am": 1, "fm": 1, "pm_bpsk": 1, "pm_qpsk": 2, "qam": 4, "fdm": 1}
TITLES = {"am": "AM / OOK", "fm": "FM / binary FSK", "pm_bpsk": "BPSK", "pm_qpsk": "QPSK", "qam": "16-QAM", "fdm": "FDM / two AM carriers"}


def basis(count, start, frequencies):
    # Integer sample indices preserve the known absolute carrier reference.
    indices = start + np.arange(count, dtype=np.int64)
    columns = []
    for frequency in frequencies:
        phase = 2 * np.pi * ((indices * frequency) % RATE) / RATE
        columns.extend((np.cos(phase), -np.sin(phase)))
    return np.column_stack(columns)


def fit(samples, start, frequencies):
    design = basis(len(samples), start, frequencies)
    coefficients = np.linalg.lstsq(design, samples, rcond=None)[0]
    residual = np.mean((samples - design @ coefficients) ** 2)
    return coefficients[::2] + 1j * coefficients[1::2], residual


def recover_fm(samples, start):
    candidates = []
    for frequency in (CARRIER - 250, CARRIER + 250):
        points, residual = fit(samples, start, [frequency])
        candidates.append((residual, frequency, points[0]))
    residual, frequency, coefficient = min(candidates, key=lambda item: item[0])
    indices = start + np.linspace(0, len(samples) - 1, 64)
    phase = 2 * np.pi * np.remainder(indices * (frequency - CARRIER), RATE) / RATE
    return coefficient * np.exp(1j * phase), frequency, residual


def recover(path, kind):
    points, tones, residuals = [], [], []
    with wave.open(str(path), "rb") as audio:
        if (audio.getnchannels(), audio.getsampwidth(), audio.getframerate(), audio.getcomptype()) != (1, 2, RATE, "NONE"):
            raise ValueError("expected mono, 16-bit PCM at 48000 Hz")
        size = SPB * SIZES[kind]
        total = audio.getnframes() // size
        if total == 0 or audio.getnframes() % size:
            raise ValueError("WAV must contain complete, nonempty symbols")
        selected = np.linspace(0, total - 1, min(4096, total), dtype=np.int64)
        for symbol in selected:
            start = int(symbol) * size
            audio.setpos(start)
            samples = np.frombuffer(audio.readframes(size), dtype="<i2").astype(float) / 32767
            if len(samples) != size:
                raise ValueError("truncated WAV")
            if kind == "fm":
                recovered, tone, residual = recover_fm(samples, start)
                tones.append(tone)
            else:
                recovered, residual = fit(samples, start, [1070, 3070] if kind == "fdm" else [1070])
            points.append(recovered)
            residuals.append(residual)
    points = np.array(points)
    if not np.isfinite(points).all():
        raise ValueError("recovery produced nonfinite points")
    return points, np.array(tones), float(np.sqrt(max(residuals))), total


def plot(path, kind):
    points, tones, error, total = recover(path, kind)
    output = path.with_name(path.stem + "-constellation.png")
    panels = 2 if kind == "fdm" else 1
    fig, axes = plt.subplots(1, panels, figsize=(12 if panels == 2 else 7, 7), squeeze=False)
    for channel, ax in enumerate(axes[0]):
        ax.set_aspect("equal")
        ax.set(xlim=(-1.15, 1.15), ylim=(-1.15, 1.15), xlabel="In-phase (I)", ylabel="Quadrature (Q)")
        ax.axhline(0, color="#bbc3ce", linewidth=0.8)
        ax.axvline(0, color="#bbc3ce", linewidth=0.8)
        ax.grid(alpha=0.2)
        if kind == "fm":
            for tone, color in [(820, "#2563eb"), (1320, "#e87924")]:
                trajectory = points[tones == tone]
                ax.scatter(trajectory.real, trajectory.imag, s=2, alpha=0.25, color=color, rasterized=True)
                ax.scatter([], [], s=30, color=color, label=f"{tone} Hz ({tone-CARRIER:+d} Hz baseband)")
            ax.legend(loc="upper right", fontsize=9)
            ax.set_title("Recovered I/Q trajectories\nFrequency changes rotate the complex envelope")
        else:
            values = points[:, channel]
            ax.scatter(values.real, values.imag, s=65, alpha=0.35, color="#2563eb", edgecolors="none")
            ax.set_title(f"{[1070, 3070][channel]} Hz carrier · mixer gain ½" if kind == "fdm" else "Recovered I/Q constellation")
    fig.suptitle(TITLES[kind], fontsize=20, fontweight="bold")
    note = "Tone fits reconstructed relative to 1070 Hz" if kind == "fm" else "Joint carrier least squares" if kind == "fdm" else "Per-symbol carrier least squares"
    fig.text(0.5, 0.065, f"{len(points):,} evenly sampled symbols / {total:,} total · 48 kHz · 100 bit/s\n{note}; known timing and carrier phase\ns(t) = I cos(2πfct) − Q sin(2πfct); no constellation snapping", ha="center", fontsize=9, color="#475569")
    fig.tight_layout(rect=(0, 0.15, 1, 0.94))
    # Exclusive creation protects plots from earlier runs.
    try:
        with output.open("xb") as destination:
            try:
                fig.savefig(destination, format="png", dpi=180)
            except Exception:
                output.unlink(missing_ok=True)
                raise
    finally:
        plt.close(fig)
    print(f"{kind}: max fit RMS error {error:.3g}; Generated: {output.resolve()}")
    return output


def self_test():
    start = 123 * SPB
    for coefficients, frequencies in [([0.3, -0.8], [1070]), ([0.1, -0.5, -0.2, 0.25], [1070, 3070])]:
        samples = basis(SPB, start, frequencies) @ coefficients
        quantized = np.round(samples * 32767) / 32767
        got, residual = fit(quantized, start, frequencies)
        expected = np.array(coefficients[::2]) + 1j * np.array(coefficients[1::2])
        assert np.allclose(got, expected, atol=1e-5)
        assert residual < 1e-9
    for tone in (820, 1320):
        coefficient = 0.7 * np.exp(0.37j)
        samples = basis(SPB, start, [tone]) @ [coefficient.real, coefficient.imag]
        trajectory, recovered_tone, residual = recover_fm(samples, start)
        indices = start + np.linspace(0, SPB - 1, 64)
        expected = coefficient * np.exp(2j * np.pi * indices * (tone - CARRIER) / RATE)
        assert recovered_tone == tone and residual < 1e-20
        assert np.allclose(trajectory, expected, atol=1e-10)
    print("I/Q recovery self-check passed")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", nargs="?", choices=SIZES)
    parser.add_argument("wav", nargs="?", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    elif args.kind and args.wav:
        plot(args.wav, args.kind)
    else:
        parser.error("provide a modulation name and WAV path, or --self-test")
