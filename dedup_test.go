package gocloc

import (
	"crypto/md5"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicateDigest(t *testing.T) {
	fileCache := make(map[string]struct{})

	if duplicateDigest(md5.Sum([]byte("same content")), fileCache) {
		t.Errorf("invalid sequence")
	}
	if !duplicateDigest(md5.Sum([]byte("same content")), fileCache) {
		t.Errorf("invalid sequence")
	}
}

func TestHashFile(t *testing.T) {
	content := []byte("package sample\n")
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	digest, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := md5.Sum(content); digest != want {
		t.Fatalf("hashFile() = %x, want %x", digest, want)
	}

	if _, err := hashFile(filepath.Join(t.TempDir(), "missing.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("hashFile() did not report a missing file")
	}
}
