"""Check the loop-based carrier conversion against the original array calculation."""

import numpy as np

import waveform


def test_downconvert():
    sample_rate = 8000
    samples = np.tile(np.array([0, 1234, -5678, 32767, -32768], dtype=np.int16), 50)
    phase = 2 * np.pi * waveform.CARRIER_HZ * np.arange(len(samples)) / sample_rate
    amplitude = 2 * samples.astype(np.float64) / 32768
    alpha = 1 / (1 + sample_rate / (2 * np.pi * waveform.BASEBAND_CUTOFF_HZ))
    expected_i = waveform.low_pass(waveform.low_pass(amplitude * np.cos(phase), alpha), alpha)
    expected_q = waveform.low_pass(waveform.low_pass(-amplitude * np.sin(phase), alpha), alpha)

    actual_i, actual_q = waveform.downconvert(samples, sample_rate)
    assert np.allclose(actual_i, expected_i)
    assert np.allclose(actual_q, expected_q)

    window = round(sample_rate / waveform.BIT_RATE)
    average_i = np.convolve(expected_i, np.ones(window) / window)[:len(samples)]
    average_q = np.convolve(expected_q, np.ones(window) / window)[:len(samples)]
    assert np.allclose(waveform.matched_filter(samples, sample_rate), average_i**2 + average_q**2)


if __name__ == "__main__":
    test_downconvert()
    print("Waveform conversion check passed")
