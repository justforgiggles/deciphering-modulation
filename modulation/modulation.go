// Package modulation defines the shared bit-to-audio contract for modulations.
package modulation

// Processor modulates unpacked bits (0 or 1) into finite audio samples in [-1, 1].
// Each call returns len(bits)*SamplesPerBit() samples without modifying bits.
// Timing is positive and fixed for the lifetime of a processor. Implementations
// preserve signal state across chunks and leave it unchanged on invalid input.
// Implementations may require complete symbols (QPSK requires an even bit count).
// 16-QAM requires the bit count to be divisible by four.
// SamplesPerBit is the average sample count per bit, including multi-bit symbols.
type Processor interface {
	Modulate(bits []byte) ([]float64, error)
	SampleRate() int
	SamplesPerBit() int
}

// BytesToBits expands each byte into eight bits, most-significant bit first.
// Leading zero bits are preserved: 0x02 becomes {0, 0, 0, 0, 0, 0, 1, 0}.
func BytesToBits(data []byte) []byte {
	bits := make([]byte, len(data)*8)
	for i, b := range data {
		for bit := 0; bit < 8; bit++ {
			bits[i*8+bit] = (b >> (7 - bit)) & 1
		}
	}
	return bits
}
