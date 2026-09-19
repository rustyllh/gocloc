//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rustyllh/gocloc"
)

func TestCLIIntegration(t *testing.T) {
	binary := integrationBinary(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		"src/main.go":         "package sample\n// comment\n\nfunc main() {}\n",
		"src/copy.go":         "package sample\n// comment\n\nfunc main() {}\n",
		"scripts/run.py":      "#!/usr/bin/env python\n# comment\n# another\n\n\nprint(1)\nprint(2)\nprint(3)\n",
		"target/generated.go": "package generated\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("statistics", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			args  []string
			files int32
			code  int32
		}{
			{name: "default", args: []string{"."}, files: 3, code: 7},
			{name: "no deduplication", args: []string{"--skip-duplicated", "."}, files: 4, code: 9},
			{name: "one worker", args: []string{"--workers=1", "."}, files: 3, code: 7},
			{name: "multiple paths", args: []string{"src", "scripts"}, files: 2, code: 6},
			{name: "interspersed", args: []string{"src", "--workers", "2", "scripts"}, files: 2, code: 6},
			{name: "exclude directory", args: []string{"--not-match-d=target", "."}, files: 2, code: 6},
			{name: "include directory", args: []string{"--match-d=scripts", "."}, files: 1, code: 4},
			{name: "include filename", args: []string{`--match=\.py$`, "."}, files: 1, code: 4},
			{name: "exclude filename", args: []string{`--not-match=\.py$`, "."}, files: 2, code: 3},
			{name: "fullpath", args: []string{"--fullpath", `--match=^scripts[/\\]`, "."}, files: 1, code: 4},
			{name: "exclude extension", args: []string{"--exclude-ext=go,txt", "."}, files: 1, code: 4},
			{name: "include languages", args: []string{"--include-lang=Python,JSON", "."}, files: 1, code: 4},
		} {
			t.Run(tc.name, func(t *testing.T) {
				args := append([]string{"--output-type=json"}, tc.args...)
				stdout, stderr, err := runCLI(binary, dir, args)
				if err != nil || stderr != "" {
					t.Fatalf("run CLI: %v, stderr: %s", err, stderr)
				}
				var result gocloc.JSONLanguagesResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if result.Total.FilesCount != tc.files || result.Total.Code != tc.code {
					t.Fatalf("total = %+v, want %d files, %d code", result.Total, tc.files, tc.code)
				}
			})
		}
	})

	t.Run("short and long options are equivalent", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			long  []string
			short []string
		}{
			{
				name:  "file output and sorting",
				long:  []string{"--workers=8", "--by-file", "--sort=name", "--output-type=json", "."},
				short: []string{"-w", "8", "-f", "-s", "name", "-o", "json", "."},
			},
			{
				name:  "attached values and combined flags",
				long:  []string{"--workers=8", "--by-file", "--sort=name", "--output-type=json", "."},
				short: []string{"-w08", "-fsname", "-ojson", "."},
			},
			{
				name:  "include and exclude",
				long:  []string{"--include-lang=Go,Python", "--exclude-ext=go,txt", "--output-type=json", "."},
				short: []string{"-l", "Go,Python", "-e", "go,txt", "-o=json", "."},
			},
			{
				name:  "mixed forms and paths",
				long:  []string{"--workers=2", "--sort=name", "--output-type=json", "src", "scripts"},
				short: []string{"src", "-w2", "--sort=name", "-ojson", "scripts"},
			},
			{name: "version", long: []string{"--version"}, short: []string{"-V"}},
			{name: "languages", long: []string{"--show-lang"}, short: []string{"-L"}},
			{name: "help", long: []string{"--help"}, short: []string{"-h"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				longOut, longErrOut, longErr := runCLI(binary, dir, tc.long)
				shortOut, shortErrOut, shortErr := runCLI(binary, dir, tc.short)
				if longErr != nil || shortErr != nil {
					t.Fatalf("long: %v %s; short: %v %s", longErr, longErrOut, shortErr, shortErrOut)
				}
				if tc.name == "languages" {
					longOut = normalizeLanguageExtensions(longOut)
					shortOut = normalizeLanguageExtensions(shortOut)
				}
				if shortOut != longOut || shortErrOut != longErrOut {
					t.Fatalf("long and short outputs differ:\n%s\n%s", longOut, shortOut)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, args := range [][]string{
			{"--unknown"}, {"--workers"}, {"--workers=0"}, {"--workers=-1", "--version"},
			{"--sort=unknown", "."}, {"--by-file", "--sort=files", "."}, {"--match=[", "."},
			{"-w0", "-V"}, {"-w65", "-L"}, {"-sunknown", "."}, {"-f", "-sfiles", "."},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				stdout, stderr, err := runCLI(binary, dir, args)
				if err == nil || stdout != "" || strings.TrimSpace(stderr) == "" {
					t.Fatalf("err = %v, stdout = %q, stderr = %q", err, stdout, stderr)
				}
				if strings.Contains(stderr, "panic:") || strings.Contains(stderr, "Usage:") {
					t.Fatalf("unexpected panic or usage: %s", stderr)
				}
				if strings.Count(strings.TrimSpace(stderr), "\n") != 0 {
					t.Fatalf("expected one error line, got %q", stderr)
				}
			})
		}
	})

	t.Run("paths resembling commands", func(t *testing.T) {
		for _, name := range []string{"help", "completion", "--version"} {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, name)
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "main.go"), []byte("package main\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				stdout, stderr, err := runCLI(binary, root, []string{"--output-type=json", "--", name})
				if err != nil || stderr != "" {
					t.Fatalf("run CLI: %v, stderr: %s", err, stderr)
				}
				var result gocloc.JSONLanguagesResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if result.Total.FilesCount != 1 {
					t.Fatalf("expected one file, got %+v", result.Total)
				}
			})
		}
	})

	t.Run("output compatibility", func(t *testing.T) {
		// Optional comparison against a CLI built before the migration.
		baseline := os.Getenv("GOCLOC_CLI_BASELINE")
		cases := []struct {
			name string
			args []string
		}{
			{name: "defaults", args: []string{"."}},
			{name: "all languages", args: []string{"--show-lang"}},
			{name: "debug", args: []string{"--debug", "src/copy.go"}},
			{name: "filters", args: []string{"--not-match-d=target", "--include-lang=Go,Python", "."}},
			{name: "no deduplication", args: []string{"--skip-duplicated", "--sort=name", "."}},
			{name: "serial", args: []string{"--workers=1", "."}},
			{name: "parallel", args: []string{"--workers=8", "."}},
			{name: "decimal leading zero", args: []string{"--workers=08", "."}},
			{name: "interspersed", args: []string{"src", "--workers", "2", "scripts"}},
			{name: "exclude extensions", args: []string{"--exclude-ext=py,txt", "."}},
			{name: "fullpath", args: []string{"--fullpath", `--match=^scripts[/\\]`, "."}},
			{name: "include directory", args: []string{"--match-d=src", "."}},
			{name: "exclude name", args: []string{`--not-match=\.py$`, "."}},
			{name: "repeated option", args: []string{"--include-lang=Go", "--include-lang=Python", "."}},
		}
		for _, format := range []string{"default", "json", "cloc-xml", "markdown", "sloccount"} {
			for _, byFile := range []bool{false, true} {
				args := []string{"--sort=name", "--output-type=" + format, "."}
				name := format
				if byFile {
					args = append([]string{"--by-file"}, args...)
					name += " by file"
				}
				cases = append(cases, struct {
					name string
					args []string
				}{name: name, args: args})
			}
		}
		for _, sort := range []string{"name", "files", "blank", "comment", "code"} {
			cases = append(cases, struct {
				name string
				args []string
			}{name: "sort " + sort, args: []string{"--sort=" + sort, "."}})
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				after, newStderr, newErr := runCLI(binary, dir, tc.args)
				if newErr != nil || after == "" {
					t.Fatalf("run CLI: %v, stdout: %q, stderr: %q", newErr, after, newStderr)
				}
				if tc.name == "debug" {
					if !strings.Contains(newStderr, "filename=") || strings.Contains(after, "filename=") {
						t.Fatalf("debug logs must be on stderr: stdout=%q stderr=%q", after, newStderr)
					}
				} else if newStderr != "" {
					t.Fatalf("unexpected diagnostics: %s", newStderr)
				}
				if baseline == "" {
					return
				}
				before, oldStderr, oldErr := runCLI(baseline, dir, tc.args)
				if oldErr != nil || newErr != nil {
					t.Fatalf("before: %v %s; after: %v %s", oldErr, oldStderr, newErr, newStderr)
				}
				if tc.name == "all languages" {
					// Extension order comes from a map and varies even between legacy runs.
					before = normalizeLanguageExtensions(before)
					after = normalizeLanguageExtensions(after)
				}
				if tc.name == "debug" {
					// Debug text is unchanged, but now precedes the report on stderr
					// rather than corrupting the statistical output on stdout.
					after = newStderr + after
					newStderr = ""
				}
				if before != after || oldStderr != newStderr {
					t.Fatalf("output changed\nbefore:\n%s\n%s\nafter:\n%s\n%s", before, oldStderr, after, newStderr)
				}
			})
		}
	})
}

func normalizeLanguageExtensions(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		name, extensions, ok := strings.Cut(line, "(")
		if !ok {
			continue
		}
		items := strings.Split(strings.TrimSuffix(extensions, ")"), ", ")
		sort.Strings(items)
		lines[i] = name + "(" + strings.Join(items, ", ") + ")"
	}
	return strings.Join(lines, "\n")
}

func runCLI(binary, dir string, args []string) (string, string, error) {
	command := exec.Command(binary, args...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
