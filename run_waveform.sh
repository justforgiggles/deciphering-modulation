#!/bin/sh
set -eu

cd "$(dirname "$0")"
mkdir -p output

waveform_tmpdir=$(mktemp -d)
trap 'rm -rf "$waveform_tmpdir"' 0
dd if=data/image-small.png of="$waveform_tmpdir/input" bs=1 count=1 2>/dev/null

for kind in am fm pm_bpsk pm_qpsk qam; do
    bits_per_symbol=1
    labels=bits
    if [ "$kind" = pm_qpsk ]; then
        bits_per_symbol=2
        labels=symbols
    elif [ "$kind" = qam ]; then
        bits_per_symbol=4
        labels=symbols
    fi
    png="output/${kind}-demo-waveform-0-0.05-${labels}.png"
    if [ -e "$png" ]; then
        printf 'Already exists: %s\n' "$png"
        continue
    fi
    go run . encode -modulation "$kind" -in "$waveform_tmpdir/input" -out "$waveform_tmpdir/$kind.wav"
    python3 waveform.py "$waveform_tmpdir/$kind.wav" 0 0.05 --source "$waveform_tmpdir/input" --bits-per-symbol "$bits_per_symbol" -o "$png"
done

for wav in output/*.wav; do
    [ -f "$wav" ] || continue
    png="${wav%.wav}-waveform-0-0.05.png"
    if [ -e "$png" ]; then
        printf 'Already exists: %s\n' "$png"
        continue
    fi
    python3 waveform.py "$wav" 0 0.05
done
