"""Find and decode a framed AM/OOK transmission in a mono PCM WAV."""

import argparse
from pathlib import Path
import zlib

import numpy as np

from audio_io import read_pcm


ROOT = Path(__file__).resolve().parent.parent
INPUT_FILE = ROOT / "output/mixed.wav"
OUTPUT_FILE = ROOT / "output/recovered_am.txt"
PREAMBLE = bytes.fromhex("a23ceefca784df4b388cc4cb53a691b1")
BIT_RATE = 100
CARRIER_HZ = 1070
BANDWIDTH_HZ = 500
BASEBAND_CUTOFF_HZ = 250


def band_pass(samples, sample_rate):
    if len(samples) == 0 or sample_rate <= 0 or not 0 < CARRIER_HZ - BANDWIDTH_HZ / 2 < CARRIER_HZ + BANDWIDTH_HZ / 2 < sample_rate / 2:
        raise ValueError("audio must be nonempty and the band must fit below Nyquist")

    angle = 2 * np.pi * CARRIER_HZ / sample_rate
    q = CARRIER_HZ / BANDWIDTH_HZ
    alpha = np.sin(angle) / (2 * q)
    a0 = 1 + alpha
    b0, b2 = alpha / a0, -alpha / a0
    a1, a2 = -2 * np.cos(angle) / a0, (1 - alpha) / a0

    filtered = np.empty(len(samples), dtype=np.float64)
    previous_input = older_input = previous_output = older_output = 0.0
    for index, sample in enumerate(samples):
        current_input = float(sample)
        current_output = b0 * current_input + b2 * older_input - a1 * previous_output - a2 * older_output
        filtered[index] = current_output
        older_input, previous_input = previous_input, current_input
        older_output, previous_output = previous_output, current_output

    peak = np.max(np.abs(filtered))
    if peak > 32767:
        filtered *= 32767 / peak
    return np.rint(filtered).astype("<i2")


def low_pass(samples, alpha):
    filtered = np.empty(len(samples), dtype=np.float64)
    previous = 0.0
    for index, sample in enumerate(samples):
        previous += alpha * (sample - previous)
        filtered[index] = previous
    return filtered


def downconvert(samples, sample_rate):
    phase = 2 * np.pi * CARRIER_HZ * np.arange(len(samples)) / sample_rate
    amplitude = 2 * samples.astype(np.float64) / 32768
    alpha = 1 / (1 + sample_rate / (2 * np.pi * BASEBAND_CUTOFF_HZ))
    in_phase = low_pass(low_pass(amplitude * np.cos(phase), alpha), alpha)
    quadrature = low_pass(low_pass(-amplitude * np.sin(phase), alpha), alpha)
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


def decode(samples, sample_rate):
    """Return payload bytes and frame start, locating both without a timing hint."""
    response = matched_filter(band_pass(samples, sample_rate), sample_rate)
    window = round(sample_rate / BIT_RATE)
    preamble_bits = np.unpackbits(np.frombuffer(PREAMBLE, dtype=np.uint8))
    minimum_bits = (len(PREAMBLE) + 8) * 8  # Length and CRC can enclose an empty payload.
    quality = np.full(len(response), -np.inf)
    on_count = np.count_nonzero(preamble_bits)
    off_count = len(preamble_bits) - on_count

    # ponytail: exhaustive phase search suits short clips; use coarse-to-fine search for long captures.
    for phase in range(window):
        symbols = response[phase + window - 1 :: window]
        count = len(symbols) - minimum_bits + 1
        if count <= 0:
            continue
        on = np.zeros(count)
        off = np.zeros(count)
        for index, bit in enumerate(preamble_bits):
            if bit:
                on += symbols[index : index + count]
            else:
                off += symbols[index : index + count]
        starts = phase + np.arange(count) * window
        quality[starts] = on / on_count - off / off_count

    for start in np.argsort(quality)[::-1]:
        if quality[start] <= 0:
            break
        prefix_scores = response[start + window - 1 + np.arange(len(PREAMBLE) * 8 + 32) * window]
        preamble_scores = prefix_scores[: len(preamble_bits)]
        on_level = np.median(preamble_scores[preamble_bits == 1])
        off_level = np.median(preamble_scores[preamble_bits == 0])
        if on_level <= off_level:
            continue
        threshold = (on_level + off_level) / 2
        prefix_bits = (prefix_scores > threshold).astype(np.uint8)
        if np.count_nonzero(prefix_bits[: len(preamble_bits)] != preamble_bits) > 8:
            continue
        length = int.from_bytes(np.packbits(prefix_bits[len(preamble_bits) :]).tobytes(), "big")
        total_bits = minimum_bits + length * 8
        if start + total_bits * window > len(response):
            continue
        bits = (response[start + window - 1 + np.arange(total_bits) * window] > threshold).astype(np.uint8)
        payload_end = (len(PREAMBLE) + 4 + length) * 8
        payload = np.packbits(bits[(len(PREAMBLE) + 4) * 8 : payload_end]).tobytes()
        checksum = int.from_bytes(np.packbits(bits[payload_end:]).tobytes(), "big")
        if zlib.crc32(payload) == checksum:
            return payload, start / sample_rate

    raise ValueError("no valid AM frame found")


def main():
    samples, sample_rate = read_pcm(INPUT_FILE)
    payload, start_seconds = decode(samples, sample_rate)
    OUTPUT_FILE.write_text(payload.decode("utf-8"), encoding="utf-8")
    print(f"Recovered {len(payload)} bytes from {start_seconds:.3f} s: {OUTPUT_FILE}")


def self_test():
    payload = b"Hi"
    frame = PREAMBLE + len(payload).to_bytes(4, "big") + payload + zlib.crc32(payload).to_bytes(4, "big")
    rate, start = 48000, 1234
    for corrupt in (False, True):
        bits = np.unpackbits(np.frombuffer(frame, dtype=np.uint8)).copy()
        if corrupt:
            bits[(len(PREAMBLE) + 4) * 8] ^= 1
        indices = np.arange(len(bits) * 480)
        signal = bits.repeat(480).astype(float) * 12000 * np.sin(2 * np.pi * CARRIER_HZ * indices / rate)
        samples = np.random.default_rng(0).normal(0, 300, start + len(signal) + 2000)
        samples[start : start + len(signal)] += signal
        samples = np.rint(np.clip(samples, -32768, 32767)).astype("<i2")
        if corrupt:
            try:
                decode(samples, rate)
            except ValueError as error:
                assert str(error) == "no valid AM frame found"
                continue
            raise AssertionError("corrupted payload passed CRC validation")
        recovered, found = decode(samples, rate)
        assert recovered == payload and abs(found - start / rate) < 0.005
    print("AM synchronization and CRC self-check passed")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    else:
        main()
