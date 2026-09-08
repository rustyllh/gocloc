package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/hhatto/gocloc"
	"github.com/jessevdk/go-flags"
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
