package main

import (
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/rustyllh/gocloc"
)

func TestVersionString(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		commit  string
		info    *debug.BuildInfo
		want    string
	}{
		{name: "missing metadata", want: "devel"},
		{name: "tag install", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, want: "v0.1.0"},
		{name: "pseudo version", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260917060000-abcdef123456"}}, want: "v0.0.0-20260917060000-abcdef123456"},
		{name: "local build", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abcdef"}}}, want: "devel (abcdef)"},
		{name: "dirty build", info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abcdef"}, {Key: "vcs.modified", Value: "true"}}}, want: "devel (abcdef-dirty)"},
		{name: "release overrides metadata", version: "v1.0.0", commit: "release", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "other"}, {Key: "vcs.modified", Value: "true"}}}, want: "v1.0.0 (release)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionString(tc.version, tc.commit, tc.info); got != tc.want {
				t.Fatalf("versionString() = %q, want %q", got, tc.want)
			}
		})
	}
}

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
	command := exec.Command("go", "run", ".", "--workers=-1", "--version")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("go run succeeded, output: %s", output)
	}
	if !strings.Contains(string(output), "--workers must be greater than or equal to zero") {
		t.Fatalf("go run output = %q, want worker validation error", output)
	}
}

func TestCmdOptionsDefaultWorkersUsesAutomaticSelection(t *testing.T) {
	var opts CmdOptions
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.ParseArgs([]string{"."}); err != nil {
		t.Fatal(err)
	}
	if opts.Workers != nil {
		t.Fatalf("default workers = %v, want omitted automatic selection", *opts.Workers)
	}
}

func workers(value int) *int {
	return &value
}
