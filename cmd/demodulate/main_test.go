package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandFailures(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.wav")
	output := filepath.Join(dir, "results")
	if err := os.WriteFile(bad, []byte("bad WAV"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"-input", bad, "-output-dir", output}, {"-input", bad, "-output-dir", output, "-carrier", "NaN"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("failure left payload output")
		}
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"-input", bad, "-output-dir", output}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatal("existing output accepted", err)
	}
}

// CSS_RECORDING makes the external MP4 fixture an explicit acceptance gate.
// A missing file/FFmpeg with this variable set is a failure, never a skip.
func TestRecordedMP4(t *testing.T) {
	path := os.Getenv("CSS_RECORDING")
	if path == "" {
		t.Skip("set CSS_RECORDING to the supplied two-run MP4")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "recovered")
	var diagnostic bytes.Buffer
	if err := run(context.Background(), []string{"-input", path, "-output-dir", output}, &diagnostic); err != nil {
		t.Fatalf("%v\n%s", err, diagnostic.String())
	}
	want, err := os.ReadFile("../../data/lorem-ipsum-1.txt")
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(output)
	if err != nil || len(files) != 2 {
		t.Fatalf("expected two independent payloads: %v %v", files, err)
	}
	for _, name := range []string{"frame-001.bin", "frame-002.bin"} {
		got, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
	t.Log(diagnostic.String())
}
