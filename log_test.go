package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRollingLogIsBounded(t *testing.T) {
	home := t.TempDir()
	logger, err := newRollingLog(home)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("x"), 600<<10)
	for range 4 {
		if _, err := logger.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"axi.log", "axi.log.1"} {
		info, err := os.Stat(filepath.Join(home, "logs", name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > maxLogSize {
			t.Fatalf("%s is too large: %d", name, info.Size())
		}
	}
}
