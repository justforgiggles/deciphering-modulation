package modulation

import (
	"slices"
	"testing"
)

func TestBytesToBits(t *testing.T) {
	if got := BytesToBits([]byte{0x02, 0x81}); !slices.Equal(got, []byte{0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1}) {
		t.Fatal("bits must be MSB-first and retain leading zeros")
	}
	input := make([]byte, 256)
	for i := range input {
		input[i] = byte(i)
	}
	original := slices.Clone(input)
	bits := BytesToBits(input)
	if len(bits) != 256*8 || !slices.Equal(input, original) {
		t.Fatal("wrong bit count or changed input")
	}
	for i, want := range input {
		var got byte
		for _, bit := range bits[i*8 : (i+1)*8] {
			if bit > 1 {
				t.Fatal("output is not binary")
			}
			got = got*2 + bit
		}
		if got != want {
			t.Fatalf("byte %d became %d", want, got)
		}
	}
	if len(BytesToBits(nil)) != 0 {
		t.Fatal("empty input produced bits")
	}
}
