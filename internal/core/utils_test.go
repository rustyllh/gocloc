package gocloc

import (
	"testing"
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
