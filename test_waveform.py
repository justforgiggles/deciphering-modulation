"""Small end-to-end check for the waveform command."""

from array import array
from pathlib import Path
import tempfile
import wave
from unittest.mock import patch

import matplotlib.pyplot as plt

from waveform import save_waveform


def main() -> None:
    with tempfile.TemporaryDirectory() as directory:
        wav_path = Path(directory) / "tone.wav"
        with wave.open(str(wav_path), "wb") as wav:
            wav.setnchannels(1)
            wav.setsampwidth(2)
            wav.setframerate(8_000)
            wav.writeframes(array("h", [0, 16_000, -16_000, 0] * 4_000).tobytes())

        png_path = wav_path.with_name("tone-waveform-0.1-1.5.png")
        save_waveform(wav_path, 0.1, 1.5, png_path)
        assert png_path.read_bytes().startswith(b"\x89PNG\r\n\x1a\n")
        short_png = Path(directory) / "short.png"
        save_waveform(wav_path, 0.1, 0.2, short_png)
        assert short_png.read_bytes().startswith(b"\x89PNG\r\n\x1a\n")
        bits_png = Path(directory) / "bits.png"
        save_waveform(wav_path, 0, 0.6, bits_png, b"\x89")
        assert bits_png.read_bytes().startswith(b"\x89PNG\r\n\x1a\n")

        for group, start, end, labels, boundaries, centers in [
            (2, 0, 0.6, ["10", "00"], [0.5], [0.25, 0.55]),
            (2, 0.3, 1.3, ["10", "00", "10"], [0.5, 1.0], [0.4, 0.75, 1.15]),
            (4, 0, 1.2, ["1000", "1001"], [1.0], [0.5, 1.1]),
            (4, 0.3, 1.3, ["1000", "1001"], [1.0], [0.65, 1.15]),
        ]:
            fig, ax = plt.subplots()
            with patch("waveform.plt.subplots", return_value=(fig, ax)):
                save_waveform(wav_path, start, end, Path(directory) / f"symbols-{group}-{start}.png",
                              b"\x89", bits_per_symbol=group)
            assert [text.get_text() for text in ax.texts] == labels
            assert [line.get_xdata()[0] for line in ax.lines[1:]] == boundaries
            for text, center in zip(ax.texts, centers):
                assert abs(text.get_position()[0] - center) < 1e-12

        for source, bits_per_symbol, end in [(None, 2, 0.6), (None, 4, 0.6), (b"\x89", 3, 0.6),
                                             (bytes(16), 2, 1.1), (bytes(32), 4, 1.1)]:
            try:
                save_waveform(wav_path, 0, end, Path(directory) / "invalid-symbols.png",
                              source, bits_per_symbol)
            except ValueError:
                pass
            else:
                raise AssertionError("invalid symbol configuration or excessive labels accepted")

        try:
            save_waveform(wav_path, 0, 0.6, Path(directory) / "wrong-source.png", b"abc")
        except ValueError as exc:
            assert "source byte count" in str(exc)
        else:
            raise AssertionError("mismatched source was accepted")

        try:
            save_waveform(wav_path, 0.1, 1.5, png_path)
        except FileExistsError:
            pass
        else:
            raise AssertionError("existing PNG was overwritten")

        try:
            save_waveform(wav_path, 1.5, 0.1, Path(directory) / "invalid.png")
        except ValueError as exc:
            assert "times must satisfy" in str(exc)
        else:
            raise AssertionError("invalid times were accepted")

        stereo_path = Path(directory) / "stereo.wav"
        with wave.open(str(stereo_path), "wb") as wav:
            wav.setnchannels(2)
            wav.setsampwidth(2)
            wav.setframerate(8_000)
            wav.writeframes(array("h", [0, 0] * 8_000).tobytes())
        try:
            save_waveform(stereo_path, 0, 0.5, Path(directory) / "stereo.png")
        except ValueError as exc:
            assert "mono, 16-bit PCM" in str(exc)
        else:
            raise AssertionError("stereo WAV was accepted")


if __name__ == "__main__":
    main()
