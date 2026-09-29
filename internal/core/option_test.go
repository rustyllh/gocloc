package gocloc

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNewClocOptionsDeduplication(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, workers := range []int{1, 8} {
		t.Run("workers="+strconv.Itoa(workers), func(t *testing.T) {
			dedupOpts := NewClocOptions()
			dedupOpts.SkipDuplicated = false
			for _, tc := range []struct {
				name  string
				opts  *ClocOptions
				files int32
			}{
				{name: "constructor counts copies", opts: NewClocOptions(), files: 2},
				{name: "explicit deduplication", opts: dedupOpts, files: 1},
				{name: "zero value retains deduplication", opts: &ClocOptions{}, files: 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					tc.opts.Workers = workers
					result := analyzeDirectory(t, dir, tc.opts)
					if result.Total.Total != tc.files || result.Total.Code != tc.files {
						t.Fatalf("total = %+v, want %d files and code lines", result.Total, tc.files)
					}
				})
			}
		})
	}
	t.Run("nil options use constructor defaults", func(t *testing.T) {
		result := analyzeDirectory(t, dir, nil)
		if result.Total.Total != 2 || result.Total.Code != 2 {
			t.Fatalf("total = %+v, want 2 files and code lines", result.Total)
		}
	})
}
