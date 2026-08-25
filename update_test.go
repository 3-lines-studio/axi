package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	for _, name := range bundleBinaries {
		data := []byte(name)
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0700, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractBundle(path, destination); err != nil {
		t.Fatal(err)
	}
	for _, name := range bundleBinaries {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, []byte(name)) {
			t.Fatalf("unexpected %s content", name)
		}
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"v0.2.0", "v0.1.9", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"latest", "v0.1.0", false},
	}
	for _, test := range cases {
		if got := newerVersion(test.candidate, test.current); got != test.want {
			t.Fatalf("%s %s: %t", test.candidate, test.current, got)
		}
	}
}

func TestVerifySHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	data := []byte("axi")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if err := verifySHA256(path, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(path, "bad"); err == nil {
		t.Fatal("invalid checksum accepted")
	}
}
