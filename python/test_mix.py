"""Small check for the signal frequency wobble."""

import numpy as np

import mix


def test_frequency_wobble():
    rate = 8000
    time = np.arange(2 * rate) / rate
    signal = np.rint(7000 * np.sin(2 * np.pi * 1000 * time)).astype("<i2")
    noise = np.full(len(signal), 10, dtype="<i2")
    mix.OFFSET_SECONDS = 0
    mix.SNR_DB = 60

    mixed = mix.process(signal, noise, rate)
    assert len(mixed) == len(signal)

    def peak_hz(center):
        window = mixed[round((center - 0.1) * rate) : round((center + 0.1) * rate)]
        return np.fft.rfftfreq(len(window), 1 / rate)[np.argmax(np.abs(np.fft.rfft(window)))]

    assert 1040 <= peak_hz(0.25) <= 1060
    assert 940 <= peak_hz(0.75) <= 960

    mix.FREQUENCY_WOBBLE_HZ = 0
    unchanged = mix.process(signal, noise, rate)
    expected = np.rint(noise + signal * (10 / np.sqrt(np.mean(signal.astype(float) ** 2))) * 1000).astype("<i2")
    assert np.array_equal(unchanged, expected)


if __name__ == "__main__":
    test_frequency_wobble()
    print("Frequency wobble check passed")
