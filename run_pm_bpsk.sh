#!/bin/sh
set -eu

cd "$(dirname "$0")"

# Rebuild so changes to the PM code, carrier, and bit rate take effect.
go build -o deciphering-modulation .
mkdir -p output
wav="output/pm_bpsk-$(date +%Y%m%d-%H%M%S)-$$.wav"
./deciphering-modulation encode -modulation pm_bpsk -in data/image-small.png -out "$wav"
printf 'Generated: %s/%s\n' "$PWD" "$wav"
