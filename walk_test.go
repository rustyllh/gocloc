package gocloc

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/spf13/afero"
)

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

// Kept only as the legacy Walk oracle; production traversal uses DirEntry.
func checkDefaultIgnore(path string, info os.FileInfo, isVCS bool) bool {
	return info.IsDir() || (!isVCS && isVCSDir(path))
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
