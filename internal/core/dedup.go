package gocloc

import "crypto/md5"

func duplicateDigest(digest [md5.Size]byte, fileCache map[string]struct{}) bool {
	c := string(digest[:])
	if _, ok := fileCache[c]; ok {
		return true
	}

	fileCache[c] = struct{}{}
	return false
}
