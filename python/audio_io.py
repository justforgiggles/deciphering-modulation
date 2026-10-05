"""Shared mono 16-bit PCM WAV file access."""

import wave

import numpy as np


def read_pcm(path):
    with wave.open(str(path), "rb") as audio:
        if (audio.getnchannels(), audio.getsampwidth(), audio.getcomptype()) != (1, 2, "NONE"):
            raise ValueError(f"{path} must be mono 16-bit PCM WAV")
        return np.frombuffer(audio.readframes(audio.getnframes()), dtype="<i2"), audio.getframerate()


def write_pcm(path, samples, sample_rate):
    with wave.open(str(path), "wb") as audio:
        audio.setnchannels(1)
        audio.setsampwidth(2)
        audio.setframerate(sample_rate)
        audio.writeframes(samples.tobytes())
