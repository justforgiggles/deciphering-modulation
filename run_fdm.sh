#!/bin/sh
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 5 ]; then
    printf 'Usage: %s FIRST SECOND [MODULATION] [CARRIER1] [CARRIER2]\n' "$0" >&2
    exit 1
fi

# Resolve relative inputs before changing to the project directory.
case "$1" in /*) first=$1 ;; *) first="$PWD/$1" ;; esac
case "$2" in /*) second=$2 ;; *) second="$PWD/$2" ;; esac
cd "$(dirname "$0")"
go build -o deciphering-modulation .
mkdir -p output
wav="output/fdm-$(date +%Y%m%d-%H%M%S)-$$.wav"
./deciphering-modulation fdm -in1 "$first" -in2 "$second" \
    -modulation "${3:-am}" -carrier1 "${4:-1070}" -carrier2 "${5:-3070}" -out "$wav"
printf 'Generated: %s/%s\n' "$PWD" "$wav"
