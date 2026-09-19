package main

import (
	"fmt"
	"runtime/debug"
)

// Version is version string for gocloc command
var Version string

// GitCommit is git commit hash string for gocloc command
var GitCommit string

func versionString(version, commit string, info *debug.BuildInfo) string {
	if info != nil {
		if version == "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		if commit == "" {
			modified := false
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					commit = setting.Value
				case "vcs.modified":
					modified = setting.Value == "true"
				}
			}
			if modified && commit != "" {
				commit += "-dirty"
			}
		}
	}
	if version == "" {
		version = "devel"
	}
	if commit == "" {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}
