//go:build integration

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Normal CI tests a local build; release CI runs the same suite against the
// checksummed archive that will be published, without rebuilding the CLI.
func integrationBinary(t *testing.T) string {
	t.Helper()
	name := "gocloc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), "bin with spaces", name)
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	releaseDir := os.Getenv("GOCLOC_RELEASE_DIR")
	if releaseDir == "" {
		if os.Getenv("GOCLOC_RELEASE_VERSION") != "" {
			t.Fatal("GOCLOC_RELEASE_VERSION requires GOCLOC_RELEASE_DIR")
		}
		build := exec.Command("go", "build", "-o", binary, ".")
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build CLI: %v\n%s", err, output)
		}
		return binary
	}

	version := strings.TrimPrefix(os.Getenv("GOCLOC_RELEASE_VERSION"), "v")
	if version == "" || strings.ContainsAny(version, `/\`) {
		t.Fatal("set GOCLOC_RELEASE_VERSION to the release version")
	}
	assets := map[string]string{
		"linux/amd64":   "gocloc_Linux_x86_64.tar.gz",
		"linux/386":     "gocloc_Linux_i386.tar.gz",
		"linux/arm64":   "gocloc_Linux_arm64.tar.gz",
		"darwin/amd64":  "gocloc_Darwin_x86_64.tar.gz",
		"darwin/arm64":  "gocloc_Darwin_arm64.tar.gz",
		"windows/amd64": "gocloc_Windows_x86_64.zip",
		"windows/386":   "gocloc_Windows_i386.zip",
	}
	manifest, err := os.ReadFile(filepath.Join(releaseDir, "gocloc_"+version+"_checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Validate every supported asset, including architectures this runner cannot execute.
	for _, asset := range assets {
		data, err := os.ReadFile(filepath.Join(releaseDir, asset))
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyReleaseChecksum(data, asset, string(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	asset, ok := assets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Fatalf("no native release target for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	data, err := os.ReadFile(filepath.Join(releaseDir, asset))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := releaseExecutable(data, name, strings.HasSuffix(asset, ".zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, executable, 0o700); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCLI(binary, t.TempDir(), []string{"--version"})
	if err != nil || stderr != "" {
		t.Fatalf("release --version: %v, stderr: %s", err, stderr)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 || fields[0] != version {
		t.Fatalf("release version = %q, want %q", stdout, version)
	}
	t.Logf("Testing release archive %s: %s", asset, strings.TrimSpace(stdout))
	return binary
}

func verifyReleaseChecksum(data []byte, asset, manifest string) error {
	want := fmt.Sprintf("%x", sha256.Sum256(data))
	matches := 0
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		matches++
		if fields[0] != want {
			return fmt.Errorf("sha256 mismatch for %s", asset)
		}
	}
	if matches != 1 {
		return fmt.Errorf("expected one checksum for %s, got %d", asset, matches)
	}
	return nil
}

// Extract only the root executable, never archive-supplied paths or symlinks.
func releaseExecutable(data []byte, name string, zipped bool) ([]byte, error) {
	var executable []byte
	matches := 0
	if zipped {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open release zip: %w", err)
		}
		for _, file := range reader.File {
			if file.Name != name {
				continue
			}
			if !file.Mode().IsRegular() {
				return nil, fmt.Errorf("%s is not a regular file", name)
			}
			stream, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in release zip: %w", name, err)
			}
			executable, err = io.ReadAll(stream)
			err = errors.Join(err, stream.Close())
			if err != nil {
				return nil, fmt.Errorf("read %s in release zip: %w", name, err)
			}
			matches++
		}
	} else {
		stream, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("open release gzip: %w", err)
		}
		// gzip.Close releases decompressor state; the underlying reader is memory.
		defer stream.Close()
		reader := tar.NewReader(stream)
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read release tar header: %w", err)
			}
			if header.Name != name {
				continue
			}
			if header.Typeflag != tar.TypeReg {
				return nil, fmt.Errorf("%s is not a regular file", name)
			}
			executable, err = io.ReadAll(reader)
			if err != nil {
				return nil, fmt.Errorf("read %s in release tar: %w", name, err)
			}
			matches++
		}
	}
	if matches != 1 || len(executable) == 0 {
		return nil, fmt.Errorf("expected one nonempty %s executable, got %d entries", name, matches)
	}
	return executable, nil
}

func TestVerifyReleaseChecksum(t *testing.T) {
	data := []byte("release archive")
	valid := fmt.Sprintf("%x  gocloc.tar.gz\n", sha256.Sum256(data))
	for _, tc := range []struct {
		name     string
		manifest string
		wantErr  bool
	}{
		{name: "valid", manifest: valid},
		{name: "CRLF", manifest: strings.ReplaceAll(valid, "\n", "\r\n")},
		{name: "missing", manifest: "", wantErr: true},
		{name: "wrong name", manifest: strings.ReplaceAll(valid, "gocloc", "other"), wantErr: true},
		{name: "duplicate", manifest: valid + valid, wantErr: true},
		{name: "corrupt", manifest: strings.Repeat("0", 64) + "  gocloc.tar.gz\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyReleaseChecksum(data, "gocloc.tar.gz", tc.manifest)
			if (err != nil) != tc.wantErr {
				t.Fatalf("verify checksum: %v, want error: %t", err, tc.wantErr)
			}
		})
	}
}

func TestReleaseExecutable(t *testing.T) {
	for _, zipped := range []bool{false, true} {
		t.Run(fmt.Sprintf("zip=%t", zipped), func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				entries []string
				symlink bool
				wantErr bool
			}{
				{name: "valid", entries: []string{"LICENSE", "gocloc"}},
				{name: "missing", entries: []string{"LICENSE"}, wantErr: true},
				{name: "duplicate", entries: []string{"gocloc", "gocloc"}, wantErr: true},
				{name: "nested path", entries: []string{"bin/gocloc"}, wantErr: true},
				{name: "traversal", entries: []string{"../gocloc"}, wantErr: true},
				{name: "symlink", entries: []string{"gocloc"}, symlink: true, wantErr: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var archive bytes.Buffer
					const content = "test executable"
					if zipped {
						writer := zip.NewWriter(&archive)
						for _, name := range tc.entries {
							header := &zip.FileHeader{Name: name}
							header.SetMode(0o700)
							if tc.symlink {
								header.SetMode(os.ModeSymlink | 0o700)
							}
							entry, err := writer.CreateHeader(header)
							if err != nil {
								t.Fatal(err)
							}
							if _, err := io.WriteString(entry, content); err != nil {
								t.Fatal(err)
							}
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
					} else {
						compressed := gzip.NewWriter(&archive)
						writer := tar.NewWriter(compressed)
						for _, name := range tc.entries {
							header := &tar.Header{Name: name, Mode: 0o700, Size: int64(len(content)), Typeflag: tar.TypeReg}
							if tc.symlink {
								header.Typeflag = tar.TypeSymlink
								header.Linkname = "elsewhere"
								header.Size = 0
							}
							if err := writer.WriteHeader(header); err != nil {
								t.Fatal(err)
							}
							if !tc.symlink {
								if _, err := io.WriteString(writer, content); err != nil {
									t.Fatal(err)
								}
							}
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
						if err := compressed.Close(); err != nil {
							t.Fatal(err)
						}
					}
					got, err := releaseExecutable(archive.Bytes(), "gocloc", zipped)
					if (err != nil) != tc.wantErr {
						t.Fatalf("extract executable: %v, want error: %t", err, tc.wantErr)
					}
					if err == nil && string(got) != content {
						t.Fatalf("extracted %q, want %q", got, content)
					}
				})
			}
			t.Run("corrupt archive", func(t *testing.T) {
				if _, err := releaseExecutable([]byte("corrupt"), "gocloc", zipped); err == nil {
					t.Fatal("accepted a corrupt archive")
				}
			})
		})
	}
}
