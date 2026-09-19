package main

import (
	"bytes"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/rustyllh/gocloc"
	"github.com/spf13/cobra"
)

func TestConfigureWorkerOptions(t *testing.T) {
	t.Run("fixed worker count", func(t *testing.T) {
		clocOpts := gocloc.NewClocOptions()
		if err := configureWorkerOptions(CmdOptions{Workers: workers(3)}, clocOpts); err != nil {
			t.Fatal(err)
		}
		if clocOpts.Workers != 3 {
			t.Fatalf("worker count = %d, want 3", clocOpts.Workers)
		}
	})

	t.Run("omitted worker count uses automatic selection", func(t *testing.T) {
		clocOpts := gocloc.NewClocOptions()
		if err := configureWorkerOptions(CmdOptions{}, clocOpts); err != nil {
			t.Fatal(err)
		}
		if clocOpts.Workers != automaticWorkerCount() {
			t.Fatalf("worker count = %d, want automatic value %d", clocOpts.Workers, automaticWorkerCount())
		}
	})

	t.Run("zero worker count is rejected", func(t *testing.T) {
		if err := configureWorkerOptions(CmdOptions{Workers: workers(0)}, gocloc.NewClocOptions()); err == nil {
			t.Fatal("configureWorkerOptions() error = nil, want error")
		}
	})

	t.Run("negative worker count is rejected", func(t *testing.T) {
		if err := configureWorkerOptions(CmdOptions{Workers: workers(-1)}, gocloc.NewClocOptions()); err == nil {
			t.Fatal("configureWorkerOptions() error = nil, want error")
		}
	})

	t.Run("excessive worker count is rejected", func(t *testing.T) {
		if err := configureWorkerOptions(CmdOptions{Workers: workers(gocloc.MaxWorkers + 1)}, gocloc.NewClocOptions()); err == nil {
			t.Fatal("configureWorkerOptions() error = nil, want error")
		}
	})
}

func TestAutomaticWorkerCountUsesRuntimeLimit(t *testing.T) {
	previous := runtime.GOMAXPROCS(9)
	defer runtime.GOMAXPROCS(previous)

	if got := automaticWorkerCount(); got != 9 {
		t.Fatalf("automaticWorkerCount() = %d, want 9", got)
	}
}

func TestNegativeWorkerOptionIsRejectedBeforeVersionExit(t *testing.T) {
	command := newRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--workers=-1", "--version"})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected worker validation error")
	}
	if !strings.Contains(err.Error(), "--workers must be greater than or equal to zero") {
		t.Fatalf("error = %q, want worker validation error", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unexpected output before handling error: %q", output.String())
	}
}

func TestCmdOptionsDefaultWorkersUsesAutomaticSelection(t *testing.T) {
	command := newRootCommand()
	if err := command.ParseFlags([]string{"."}); err != nil {
		t.Fatal(err)
	}
	if command.Flags().Changed("workers") {
		t.Fatal("workers must remain omitted for automatic selection")
	}
}

func TestRootCommandParsing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		paths   []string
		workers int
	}{
		{name: "equals", args: []string{"--workers=8", "src", "test"}, paths: []string{"src", "test"}, workers: 8},
		{name: "decimal leading zero", args: []string{"--workers=08", "src"}, paths: []string{"src"}, workers: 8},
		{name: "space", args: []string{"--workers", "4", "src"}, paths: []string{"src"}, workers: 4},
		{name: "short space", args: []string{"-w", "8", "src"}, paths: []string{"src"}, workers: 8},
		{name: "short equals", args: []string{"-w=8", "src"}, paths: []string{"src"}, workers: 8},
		{name: "short attached", args: []string{"-w08", "src"}, paths: []string{"src"}, workers: 8},
		{name: "mixed last value", args: []string{"--workers=4", "src", "-w8"}, paths: []string{"src"}, workers: 8},
		{name: "interspersed", args: []string{"src", "--workers=2", "test"}, paths: []string{"src", "test"}, workers: 2},
		{name: "terminator", args: []string{"--workers=1", "--", "--version"}, paths: []string{"--version"}, workers: 1},
		{name: "command-like paths", args: []string{"help", "completion"}, paths: []string{"help", "completion"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := newRootCommand()
			command.SetArgs(tc.args)
			called := false
			command.RunE = func(cmd *cobra.Command, paths []string) error {
				called = true
				if !reflect.DeepEqual(paths, tc.paths) {
					t.Fatalf("paths = %v, want %v", paths, tc.paths)
				}
				workers, err := cmd.Flags().GetInt("workers")
				if err != nil {
					t.Fatal(err)
				}
				if workers != tc.workers {
					t.Fatalf("workers = %d, want %d", workers, tc.workers)
				}
				return nil
			}
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("path was treated as a subcommand")
			}
		})
	}
}

func TestRootCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--unknown"}, want: "unknown flag"},
		{name: "missing value", args: []string{"--workers"}, want: "needs an argument"},
		{name: "invalid integer", args: []string{"--workers=many"}, want: "invalid argument"},
		{name: "hexadecimal integer", args: []string{"--workers=0x8"}, want: "invalid argument"},
		{name: "zero workers", args: []string{"--workers=0", "--version"}, want: "between 1 and 64"},
		{name: "too many workers", args: []string{"--workers=65", "--show-lang"}, want: "must not exceed 64"},
		{name: "short zero workers", args: []string{"-w0", "-V"}, want: "between 1 and 64"},
		{name: "short negative workers", args: []string{"-w", "-1", "-V"}, want: "greater than or equal to zero"},
		{name: "short excessive workers", args: []string{"-w65", "-L"}, want: "must not exceed 64"},
		{name: "short missing value", args: []string{"-w"}, want: "needs an argument"},
		{name: "sort before version", args: []string{"--sort=invalid", "--version"}, want: "must be one of"},
		{name: "invalid repeated sort", args: []string{"--sort=invalid", "--sort=name", "."}, want: "must be one of"},
		{name: "sort conflict", args: []string{"--by-file", "--sort=files", "."}, want: "cannot be used"},
		{name: "short sort conflict", args: []string{"-f", "-s", "files", "."}, want: "cannot be used"},
		{name: "short sort before version", args: []string{"-sunknown", "-V"}, want: "must be one of"},
		{name: "match regex", args: []string{"--match=[", "."}, want: "invalid --match:"},
		{name: "not-match regex", args: []string{"--not-match=[", "."}, want: "invalid --not-match:"},
		{name: "match-d regex", args: []string{"--match-d=[", "."}, want: "invalid --match-d:"},
		{name: "not-match-d regex", args: []string{"--not-match-d=[", "."}, want: "invalid --not-match-d:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := newRootCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs(tc.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if output.Len() != 0 {
				t.Fatalf("error must be returned without printing: %q", output.String())
			}
		})
	}
}

func TestRootCommandInformation(t *testing.T) {
	info, _ := debug.ReadBuildInfo()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "no paths", args: []string{}, want: "Usage:"},
		{name: "help", args: []string{"--help"}, want: "--workers"},
		{name: "short help", args: []string{"-h"}, want: "Usage:"},
		{name: "version", args: []string{"--version"}, want: versionString(Version, GitCommit, info)},
		{name: "short version", args: []string{"-V"}, want: versionString(Version, GitCommit, info)},
		{name: "languages", args: []string{"--show-lang"}, want: "Go"},
		{name: "short languages", args: []string{"-L"}, want: "Go"},
		{name: "one worker", args: []string{"--workers=1", "--version"}, want: versionString(Version, GitCommit, info)},
		{name: "64 workers", args: []string{"--workers=64", "--version"}, want: versionString(Version, GitCommit, info)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := newRootCommand()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs(tc.args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), tc.want) || stderr.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q, want %q", stdout.String(), stderr.String(), tc.want)
			}
		})
	}
}

func workers(value int) *int {
	return &value
}
