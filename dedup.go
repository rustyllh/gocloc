package gocloc

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"os"
)

func duplicateDigest(digest [md5.Size]byte, fileCache map[string]struct{}) bool {
	c := string(digest[:])
	if _, ok := fileCache[c]; ok {
		return true
	}

	fileCache[c] = struct{}{}
	return false
}

func hashFile(path string) (digest [md5.Size]byte, err error) {
	file, err := os.Open(path)
	if err != nil {
		return digest, err
	}

	hash := md5.New()
	_, readErr := io.Copy(hash, file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return digest, fmt.Errorf("hash %q: %w", path, err)
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
