package main

import (
	"runtime/debug"
	"testing"
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
