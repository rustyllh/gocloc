package lexer

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestScannerNestedBlock(t *testing.T) {
	t.Parallel()
	rules := &Rules{
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: true}},
	}
	scanner := NewScanner(rules)
	comment := []byte("/* outer /* inner */ still outer */\n")
	if scanner.ScanLine(comment, comment) || scanner.InComment() {
		t.Fatal("nested block should end as one comment line")
	}
	code := []byte("code\n")
	if !scanner.ScanLine(code, code) || scanner.InComment() {
		t.Fatal("code after nested comment was not counted")
	}
}

func TestScannerMixedNestedBlocks(t *testing.T) {
	t.Parallel()
	rules := &Rules{blockComments: []blockCommentRule{
		{open: "(*", close: "*)", nested: true},
		{open: "/*", close: "*/", nested: true},
	}}
	assertScannerKinds(t, rules, "(* outer\n/* inner\n*) ignored in inner\n*/\nstill outer\n*) code\n", "MMMMMC")
}

func TestScannerLongestMarkers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		rules  *Rules
		source string
		kinds  string
	}{
		{name: "block outranks shorter line comment", rules: &Rules{
			lineComments: []string{"#"}, blockComments: []blockCommentRule{{open: "###", close: "###"}},
		}, source: "### block\ntext\n### code\n", kinds: "MMC"},
		{name: "quote outranks shorter comment", rules: &Rules{
			lineComments: []string{"@"}, strings: []stringRule{{open: "@\"", close: "\"", multiline: true}},
		}, source: "@\"string\n@ text\n\"\n@ real\n", kinds: "CCCM"},
		{name: "documentation outranks line marker", rules: upstreamSyntax["Raku"],
			source: "#|{ docs\n# docs\n} code\n# real\n", kinds: "MMCM"},
		{name: "unicode comment marker", rules: upstreamSyntax["Apl"],
			source: "⍝ comment\n'⍝ string'\n", kinds: "MC"},
		{name: "case insensitive block markers", rules: upstreamSyntax["Autoit"],
			source: "#CS\nbody\n#CE code\n", kinds: "MMC"},
		{name: "catalog multiline raw quotes", rules: upstreamSyntax["Cue"],
			source: "x: #\"text\\\n// literal\n\"#\n// real\n", kinds: "CCCM"},
		{name: "documentation closure followed by code", rules: upstreamSyntax["Pony"],
			source: "\"\"\"docs\n\"\"\" code\n// real\n", kinds: "MCM"},
		{name: "nested opener outranks shorter closer", rules: &Rules{
			blockComments: []blockCommentRule{
				{open: "{", close: "}", nested: true},
				{open: "}{", close: "}", nested: true},
			},
		}, source: "{ }{ inner\n}\nstill outer\n} code\n", kinds: "MMMC"},
		{name: "non-nested opener cannot mask nested opener", rules: &Rules{
			blockComments: []blockCommentRule{
				{open: "/*", close: "*/", nested: true},
				{open: "/**", close: "END"},
			},
		}, source: "/* /** inner\n*/\nstill outer\n*/ code\n", kinds: "MMMC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertScannerKinds(t, test.rules, test.source, test.kinds)
		})
	}
}

func TestGeneratedRuleCatalog(t *testing.T) {
	t.Parallel()
	if len(upstreamSyntax) != 333 {
		t.Fatalf("catalog has %d entries, want 333", len(upstreamSyntax))
	}
	for name, rules := range upstreamSyntax {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if rules == nil {
				t.Fatal("catalog entry has no rules")
			}
			// Even malformed input must terminate and use fresh per-file state.
			source := "\xff \" ' /* # {- <!-- @\" ''' `\n\n*/ */ \" } -->\n"
			first := scanKinds(rules, source)
			second := scanKinds(rules, source)
			if first != second || len(first) != 3 {
				t.Fatalf("state escaped one scanner: %q vs %q", first, second)
			}
		})
	}
}

func FuzzScannerCatalog(f *testing.F) {
	names := make([]string, 0, len(upstreamSyntax))
	for name := range upstreamSyntax {
		names = append(names, name)
	}
	slices.Sort(names)
	for i := range names {
		f.Add("/* # @\" ''' (*\n\nend */\n", uint16(i))
	}
	f.Fuzz(func(t *testing.T, source string, index uint16) {
		if len(source) > 1024*1024 {
			t.Skip()
		}
		rules := upstreamSyntax[names[int(index)%len(names)]]
		kinds := scanKinds(rules, source)
		if source == "" {
			if kinds != "" {
				t.Fatal("empty source emitted a line")
			}
			return
		}
		lines := strings.Count(strings.TrimSuffix(source, "\n"), "\n") + 1
		if len(kinds) != lines || scanKinds(rules, source) != kinds {
			t.Fatalf("unstable physical-line classification: %q", kinds)
		}
	})
}

func assertScannerKinds(t *testing.T, rules *Rules, source, want string) {
	t.Helper()
	if got := scanKinds(rules, source); got != want {
		t.Fatalf("line kinds = %s, want %s", got, want)
	}
}

func scanKinds(rules *Rules, source string) string {
	if source == "" {
		return ""
	}
	scanner := NewScanner(rules)
	var kinds strings.Builder
	lines := strings.SplitAfter(source, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		input := []byte(line)
		trimmed := bytes.TrimSpace(input)
		code := scanner.ScanLine(input, trimmed)
		switch {
		case len(trimmed) == 0:
			kinds.WriteByte('B')
		case code:
			kinds.WriteByte('C')
		default:
			kinds.WriteByte('M')
		}
	}
	return kinds.String()
}
