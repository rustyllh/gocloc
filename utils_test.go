package gocloc

import (
	"crypto/md5"
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
	files, err := getAllFilesParallelMD5([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, different}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
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
