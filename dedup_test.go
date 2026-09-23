package gocloc

import (
	"crypto/md5"
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
