package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"
)

const sourcePath = "languages.json"
const outputPath = "../lexer/rules_generated.go"
const sourceSHA256 = "596748f92a5dca4065cc73e9379cf45615f1c06ba0b5b279b5851edd8258bd05"

type upstreamRule struct {
	LineComments   []string   `json:"line_comment"`
	BlockComments  [][]string `json:"multi_line_comments"`
	NestedComments [][]string `json:"nested_comments"`
	Quotes         [][]string `json:"quotes"`
	VerbatimQuotes [][]string `json:"verbatim_quotes"`
	DocQuotes      [][]string `json:"doc_quotes"`
	Nested         bool       `json:"nested"`
	Blank          bool       `json:"blank"`
	Literate       bool       `json:"literate"`
	Important      []string   `json:"important_syntax"`
}

type upstreamRules struct {
	Languages map[string]upstreamRule `json:"languages"`
}

var selected = []struct {
	name, source string
}{
	{name: "C", source: "C"},
	{name: "C Header", source: "C"},
	{name: "D", source: "D"},
	{name: "Go", source: "Go"},
	{name: "JavaScript", source: "JavaScript"},
	{name: "TypeScript", source: "TypeScript"},
}

func main() {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		fail(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != sourceSHA256 {
		fail(fmt.Errorf("unexpected tokei rules SHA-256: %s", got))
	}
	var upstream upstreamRules
	if err := json.Unmarshal(content, &upstream); err != nil {
		fail(err)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].name < selected[j].name })
	var out bytes.Buffer
	out.WriteString("// Code generated from internal/languagerules/languages.json; DO NOT EDIT.\n")
	out.WriteString("package lexer\n\nvar builtInSyntax = map[string]*Rules{\n")
	for _, language := range selected {
		rule, found := upstream.Languages[language.source]
		if !found || len(rule.LineComments) == 0 || len(rule.BlockComments) == 0 {
			fail(fmt.Errorf("incomplete rule for %q", language.source))
		}
		if rule.Nested || rule.Blank || rule.Literate || len(rule.DocQuotes) != 0 ||
			len(rule.VerbatimQuotes) != 0 || len(rule.Important) != 0 {
			fail(fmt.Errorf("unsupported special lexical fields for %q", language.source))
		}
		if language.name == "Go" || language.name == "C" || language.name == "C Header" {
			if len(rule.LineComments) != 1 || rule.LineComments[0] != "//" ||
				len(rule.BlockComments) != 1 || len(rule.BlockComments[0]) != 2 ||
				rule.BlockComments[0][0] != "/*" || rule.BlockComments[0][1] != "*/" ||
				len(rule.NestedComments) != 0 {
				fail(fmt.Errorf("fast path requires // and /* */ rules for %q", language.source))
			}
		}
		starts := validateMarkers(language.name, rule)
		fmt.Fprintf(&out, "// %s: tokei %s", language.name, language.source)
		if language.name != language.source {
			out.WriteString(" (explicit name mapping)")
		}
		switch language.name {
		case "Go":
			out.WriteString("; local rune and raw-string rules")
		case "C", "C Header":
			out.WriteString("; local character-literal and line-splice rules")
		case "JavaScript", "TypeScript":
			out.WriteString("; local template and regex-literal rules")
		}
		out.WriteByte('\n')
		fmt.Fprintf(&out, "%q: {\nstarts:[4]uint64{%d,%d,%d,%d},\nlineComments: []string{",
			language.name, starts[0], starts[1], starts[2], starts[3])
		for _, marker := range rule.LineComments {
			if marker == "" {
				fail(fmt.Errorf("empty line comment for %q", language.source))
			}
			fmt.Fprintf(&out, "%q,", marker)
		}
		out.WriteString("},\nblockComments: []blockCommentRule{")
		for _, group := range []struct {
			pairs  [][]string
			nested bool
		}{{rule.BlockComments, false}, {rule.NestedComments, true}} {
			for _, pair := range group.pairs {
				if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
					fail(fmt.Errorf("invalid block comment for %q: %q", language.source, pair))
				}
				fmt.Fprintf(&out, "{open:%q,close:%q,nested:%t},", pair[0], pair[1], group.nested)
			}
		}
		out.WriteString("},\nstrings: []stringRule{")
		for _, pair := range rule.Quotes {
			if len(pair) != 2 {
				fail(fmt.Errorf("invalid quote for %q: %q", language.source, pair))
			}
			open := strings.ReplaceAll(pair[0], `\"`, `"`)
			close := strings.ReplaceAll(pair[1], `\"`, `"`)
			if open == "" || close == "" {
				fail(fmt.Errorf("empty quote for %q", language.source))
			}
			if open == "`" && (language.name == "JavaScript" || language.name == "TypeScript") {
				continue
			}
			fmt.Fprintf(&out, "{open:%q,close:%q,escape:'\\\\'},", open, close)
		}
		if language.name == "Go" || language.name == "C" || language.name == "C Header" {
			out.WriteString("{open:\"'\",close:\"'\",escape:'\\\\'},")
		}
		if language.name == "Go" {
			out.WriteString("{open:\"`\",close:\"`\",multiline:true},")
		}
		if language.name == "JavaScript" || language.name == "TypeScript" {
			// Template literals need interpolation-aware scanning, not the generic
			// fixed-delimiter string rule generated from the upstream entry.
			out.WriteString("{open:\"`\",close:\"`\",escape:'\\\\',multiline:true},")
		}
		out.WriteString("},\n")
		if language.name == "Go" || language.name == "C" || language.name == "C Header" {
			out.WriteString("fastSimple:true,\n")
		}
		if language.name == "Go" {
			out.WriteString("rawDelimiter:'`',\n")
		}
		if language.name == "C" || language.name == "C Header" {
			out.WriteString("lineSplice:true,\n")
		}
		if language.name == "JavaScript" || language.name == "TypeScript" {
			out.WriteString("special:syntaxJavaScript,\n")
		}
		out.WriteString("},\n")
	}
	out.WriteString("}\n")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		fail(fmt.Errorf("format generated rules: %w\n%s", err, &out))
	}
	if err := os.WriteFile(outputPath, formatted, 0644); err != nil {
		fail(err)
	}
}

// Runtime dispatch checks line comments, block comments, then strings. Reject
// overlapping starts rather than making that order an accidental language rule.
func validateMarkers(name string, rule upstreamRule) [4]uint64 {
	var starts []string
	var firstBytes [4]uint64
	add := func(marker string) {
		if marker == "" {
			fail(fmt.Errorf("empty opening marker for %q", name))
		}
		for _, previous := range starts {
			if strings.HasPrefix(marker, previous) || strings.HasPrefix(previous, marker) {
				fail(fmt.Errorf("ambiguous opening markers %q and %q for %q", previous, marker, name))
			}
		}
		starts = append(starts, marker)
		first := marker[0]
		firstBytes[first>>6] |= 1 << (first & 63)
	}
	for _, marker := range rule.LineComments {
		add(marker)
	}
	for _, group := range [][][]string{rule.BlockComments, rule.NestedComments, rule.Quotes} {
		for _, pair := range group {
			if len(pair) != 2 || pair[1] == "" {
				fail(fmt.Errorf("invalid delimiter pair %q for %q", pair, name))
			}
			add(strings.ReplaceAll(pair[0], `\"`, `"`))
		}
	}
	if name == "Go" || name == "C" || name == "C Header" {
		add("'")
	}
	if name == "Go" {
		add("`")
	}
	return firstBytes
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
