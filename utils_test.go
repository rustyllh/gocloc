package gocloc

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestContainsComment(t *testing.T) {
	if !containsComment(`int a; /* A takes care of counts */`, [][]string{{"/*", "*/"}}) {
		t.Errorf("invalid")
	}
	if !containsComment(`bool f; /* `, [][]string{{"/*", "*/"}}) {
		t.Errorf("invalid")
	}
	if containsComment(`}`, [][]string{{"/*", "*/"}}) {
		t.Errorf("invalid")
	}
}

func TestCheckMD5SumIgnore(t *testing.T) {
	fileCache := make(map[string]struct{})

	if checkMD5Sum("./utils_test.go", fileCache) {
		t.Errorf("invalid sequence")
	}
	if !checkMD5Sum("./utils_test.go", fileCache) {
		t.Errorf("invalid sequence")
	}
}

func TestHashFile(t *testing.T) {
	content := []byte("package sample\n")
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	digest, ignored := hashFile(path)
	if ignored {
		t.Fatal("hashFile() ignored a readable file")
	}
	if want := md5.Sum(content); digest != want {
		t.Fatalf("hashFile() = %x, want %x", digest, want)
	}

	if _, ignored := hashFile(filepath.Join(t.TempDir(), "missing.go")); !ignored {
		t.Fatal("hashFile() did not ignore a missing file")
	}
}

func TestParallelResultWindowSize(t *testing.T) {
	for workers, want := range map[int]int{
		1:  256,
		8:  256,
		16: 512,
	} {
		if got := parallelResultWindowSize(workers); got != want {
			t.Fatalf("parallelResultWindowSize(%d) = %d, want %d", workers, got, want)
		}
	}
}

func TestGetAllFilesParallelMD5PreservesFirstFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	different := filepath.Join(dir, "c.go")
	for path, content := range map[string]string{
		first:     "package first\n",
		duplicate: "package first\n",
		different: "package different\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	opts := NewClocOptions()
	opts.Workers = 4
	files, clocFiles, err := scanAndAnalyzeFiles([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, different}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
	if len(clocFiles) != 2 || clocFiles[duplicate] != nil {
		t.Fatalf("analyzed files = %v, want only retained files", clocFiles)
	}
}

func TestParallelDetectionPreservesOrderAcrossIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	// More ignored candidates than the window must still release all tokens.
	for i := range parallelResultWindowSize(4) + 1 {
		path := filepath.Join(dir, fmt.Sprintf("a%04d.unknown", i))
		if err := os.WriteFile(path, []byte("unknown\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("#!/usr/bin/env python\n# comment\nprint(1)\n")
	for _, name := range []string{"b.go", "c.py"} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	serial := analyzeDirectory(t, dir, &ClocOptions{Workers: 1})
	parallel := analyzeDirectory(t, dir, &ClocOptions{Workers: 4})
	if !reflect.DeepEqual(serial.Files, parallel.Files) || !reflect.DeepEqual(serial.Total, parallel.Total) {
		t.Fatalf("serial and parallel results differ: %v / %v", serial.Files, parallel.Files)
	}
	first := parallel.Files[filepath.Join(dir, "b.go")]
	if len(parallel.Files) != 1 || first == nil || first.Lang != "Python" {
		t.Fatalf("expected first copy with shebang language, got %v", parallel.Files)
	}
}

func TestWalkCandidateFilesMatchesLegacyWalk(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"main.go", "target/a.go", "target/sub/b.go", "target-other/c.go",
		".git/config.go", ".hidden/visible.go", "src/keep.go", "src/skip.txt",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "src"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"", "target", "target$", `target\b`, "target|src", "(?m)target$"} {
		for _, root := range []string{dir, filepath.Join(dir, ".git"), filepath.Join(dir, "main.go")} {
			t.Run(pattern+"/"+filepath.Base(root), func(t *testing.T) {
				opts := NewClocOptions()
				if pattern != "" {
					opts.ReNotMatchDir = regexp.MustCompile(pattern)
				}
				want := []string{}
				err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !checkDefaultIgnore(path, info, isVCSDir(root)) && checkOptionMatch(path, info, opts) {
						want = append(want, path)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				got := []string{}
				if err := walkCandidateFiles(root, opts, func(path string) error {
					got = append(got, path)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("candidates = %v, legacy = %v", got, want)
				}
			})
		}
	}
}

func TestCanPruneDirectory(t *testing.T) {
	for pattern, want := range map[string]bool{
		"dist|node_modules|target": true,
		"^/src/target":             true,
		"target$":                  false,
		`target\b`:                 false,
		`target\B`:                 false,
		"(?m)target$":              false,
	} {
		if got := canPruneDirectory(regexp.MustCompile(pattern)); got != want {
			t.Errorf("canPruneDirectory(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestGetAllFilesParallelMD5ContinuesAfterWalkError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "valid.go")
	if err := os.WriteFile(file, []byte("package valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := NewClocOptions()
	opts.Workers = 4
	files, err := getAllFiles([]string{filepath.Join(dir, "missing"), dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatalf("getAllFiles() error = %v, want nil", err)
	}
	if got, want := files["Go"].Files, []string{file}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}

func TestGetAllFilesSkipDuplicatedKeepsAllFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	for _, path := range []string{first, duplicate} {
		if err := os.WriteFile(path, []byte("package same\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	opts := NewClocOptions()
	opts.Workers = 4
	opts.SkipDuplicated = true
	files, err := getAllFiles([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, duplicate}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}

func TestCheckDefaultIgnore(t *testing.T) {
	appFS := afero.NewMemMapFs()
	if err := appFS.Mkdir("/test", os.ModeDir); err != nil {
		t.Fatal(err)
	}
	_, _ = appFS.Create("/test/one.go")

	fileInfo, _ := appFS.Stat("/")
	if !checkDefaultIgnore("/", fileInfo, false) {
		t.Errorf("invalid logic: this is directory")
	}

	if !checkDefaultIgnore("/", fileInfo, true) {
		t.Errorf("invalid logic: this is vcs file or directory")
	}

	fileInfo, _ = appFS.Stat("/test/one.go")
	if checkDefaultIgnore("/test/one.go", fileInfo, false) {
		t.Errorf("invalid logic: should not ignore this file")
	}
}

type MockFileInfo struct {
	FileName    string
	IsDirectory bool
}

func (mfi MockFileInfo) Name() string       { return mfi.FileName }
func (mfi MockFileInfo) Size() int64        { return int64(8) }
func (mfi MockFileInfo) Mode() os.FileMode  { return os.ModePerm }
func (mfi MockFileInfo) ModTime() time.Time { return time.Now() }
func (mfi MockFileInfo) IsDir() bool        { return mfi.IsDirectory }
func (mfi MockFileInfo) Sys() interface{}   { return nil }

func TestCheckOptionMatch(t *testing.T) {
	opts := &ClocOptions{}
	fi := MockFileInfo{FileName: "/", IsDirectory: true}
	if !checkOptionMatch("/", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is nil")
	}

	opts.ReNotMatchDir = regexp.MustCompile("thisisdir-not-match")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is nil")
	}

	opts.ReNotMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is ignore")
	}

	opts = &ClocOptions{}
	opts.ReMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is not ignore")
	}

	opts.ReMatchDir = regexp.MustCompile("thisisdir-not-match")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is ignore")
	}

	opts = &ClocOptions{}
	opts.ReNotMatchDir = regexp.MustCompile("thisisdir-not-match")
	opts.ReMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is not ignore")
	}

	t.Run("--match option", func(t *testing.T) {
		opts = &ClocOptions{
			ReMatch: regexp.MustCompile("app.py"),
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if !checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: match is not ignore")
		}
	})

	t.Run("--match option with --fullpath option", func(t *testing.T) {
		opts = &ClocOptions{
			ReMatch:  regexp.MustCompile("test_dir/app.py"),
			Fullpath: true,
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if !checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: match(with fullpath) is not ignore")
		}
		if checkOptionMatch("app.py", fi, opts) {
			t.Errorf("invalid logic: match(with fullpath) is ignore")
		}
	})

	t.Run("--not-match option with --fullpath option", func(t *testing.T) {
		opts = &ClocOptions{
			ReNotMatch: regexp.MustCompile("test_dir/app.py"),
			Fullpath:   true,
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: not-match(with fullpath) is ignore")
		}
		if !checkOptionMatch("app.py", fi, opts) {
			t.Errorf("invalid logic: not-match(with fullpath) is not ignore")
		}
	})
}
