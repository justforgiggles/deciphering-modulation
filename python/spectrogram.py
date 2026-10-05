"""Plot a mono PCM WAV as a 0–4 kHz spectrogram."""

from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

from audio_io import read_pcm


ROOT = Path(__file__).resolve().parent.parent
INPUT_FILE = ROOT / "output/mixed.wav"
OUTPUT_FILE = ROOT / "output/spectrogram_mixed.png"


def main():
    samples, sample_rate = read_pcm(INPUT_FILE)
    fig, ax = plt.subplots(figsize=(12, 5))
    _, _, _, image = ax.specgram(
        samples.astype(float) / 32768,
        Fs=sample_rate,
        NFFT=1024,
        noverlap=768,
        cmap="magma",
        scale="dB",
    )
    ax.set(xlabel="Time (s)", ylabel="Frequency (Hz)", ylim=(0, min(4000, sample_rate / 2)))
    fig.colorbar(image, ax=ax, label="Power (dB)")
    fig.tight_layout()
    fig.savefig(OUTPUT_FILE, dpi=150)
    plt.close(fig)
    print(f"Wrote {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
