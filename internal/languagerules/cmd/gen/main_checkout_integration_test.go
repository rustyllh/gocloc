//go:build integration

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLanguageRuleCheckout(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required to test checkout line endings")
	}
	repository := t.TempDir()
	checkout := t.TempDir()
	files := map[string][]byte{"control.txt": []byte("control\n")}
	for _, path := range []string{
		".gitattributes",
		"internal/languagerules/languages.json",
		"internal/lexer/rules_generated.go",
		"internal/languagerules/COVERAGE.md",
	} {
		content, err := os.ReadFile(filepath.Join("../../../..", path))
		if path == ".gitattributes" && errors.Is(err, os.ErrNotExist) {
			// Missing attributes must exercise checkout and fail, not skip this test.
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		files[path] = content
		path := filepath.Join(repository, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "control.txt"), files["control.txt"], 0600); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		settings := []string{
			"-c", "core.autocrlf=true",
			"-c", "core.safecrlf=false",
			"-c", "core.attributesfile=" + os.DevNull,
		}
		command := exec.Command(git, append(settings, args...)...)
		command.Dir = repository
		command.Env = append(os.Environ(), "GIT_ATTR_NOSYSTEM=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf(
				"git %v: %v\n%s",
				args,
				err,
				output,
			)
		}
	}
	runGit("init", "--quiet")
	runGit("add", "--all")
	runGit("checkout-index", "--all", "--prefix="+filepath.ToSlash(checkout)+"/")
	for _, path := range []string{
		"internal/languagerules/languages.json",
		"internal/lexer/rules_generated.go",
		"internal/languagerules/COVERAGE.md",
		"control.txt",
	} {
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(checkout, path))
			if err != nil {
				t.Fatal(err)
			}
			if path == "internal/languagerules/languages.json" {
				if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != sourceSHA256 {
					t.Fatalf("snapshot checkout checksum = %s, want %s", got, sourceSHA256)
				}
			}
			want := files[path]
			if path == "control.txt" {
				want = []byte("control\r\n") // Prove that CRLF conversion is active.
			}
			if !bytes.Equal(content, want) {
				t.Fatal("checkout changed the expected file bytes")
			}
		})
	}
}
