package main

import (
	"bytes"
	"fmt"
	"strings"
)

type blockRule struct {
	open, close string
	nested      bool
	lineStart   bool
}

type quoteRule struct {
	open, close string
	escape      rune
	multiline   bool
	doubled     bool
}

type compiledRule struct {
	lines        []string
	blocks       []blockRule
	quotes       []quoteRule
	docs         []quoteRule
	starts       [4]uint64
	fastSimple   bool
	rawDelimiter rune
	lineSplice   bool
	continuation bool
	fortran      bool
	foldCase     bool
	ambiguous    bool
	special      string
	raw          string
	note         string
}

func compileRule(name string, input upstreamRule) (compiledRule, error) {
	rule := compiledRule{
		lines: []string{}, blocks: []blockRule{}, quotes: []quoteRule{}, docs: []quoteRule{},
		special: "syntaxGeneric", raw: "rawNone", note: "tokei " + name,
	}
	for _, marker := range input.LineComments {
		if marker == "" {
			return rule, fmt.Errorf("empty line-comment marker")
		}
		rule.lines = append(rule.lines, decodeMarker(marker))
	}
	for _, group := range []struct {
		pairs  [][]string
		nested bool
	}{
		{pairs: input.BlockComments, nested: input.Nested},
		{pairs: input.NestedComments, nested: true},
	} {
		for _, pair := range group.pairs {
			open, close, err := quotePair(pair)
			if err != nil {
				return rule, err
			}
			if group.nested && open == close {
				return rule, fmt.Errorf("identical nested block delimiters %q", open)
			}
			rule.blocks = append(rule.blocks, blockRule{
				open: open, close: close, nested: group.nested,
				lineStart: name == "Ruby" || name == "Perl",
			})
		}
	}
	for _, pair := range input.Quotes {
		open, close, err := quotePair(pair)
		if err != nil {
			return rule, err
		}
		rule.quotes = append(rule.quotes, quoteRule{
			open: open, close: close, escape: '\\',
			multiline: ordinaryMultiline(name, open), doubled: doubledQuotes(name),
		})
	}
	for _, pair := range input.VerbatimQuotes {
		open, close, err := quotePair(pair)
		if err != nil {
			return rule, err
		}
		if name == "Rust" || name == "Cpp" || name == "CppModule" {
			continue // Dynamic delimiters are covered by the existing scanner.
		}
		rule.quotes = append(rule.quotes, quoteRule{
			open: open, close: close, multiline: true,
			doubled: name == "CSharp" || name == "FSharp" || name == "Razor" || name == "ZenCode",
		})
	}
	for _, pair := range input.DocQuotes {
		open, close, err := quotePair(pair)
		if err != nil {
			return rule, err
		}
		if name == "Raku" {
			// Raku's POD and #| forms are documentation comments, not strings.
			rule.blocks = append(rule.blocks, blockRule{open: open, close: close})
			continue
		}
		rule.docs = append(rule.docs, quoteRule{open: open, close: close, escape: '\\', multiline: true})
	}
	applyLocalRules(name, &rule)
	if rule.fastSimple {
		if err := rule.validateFastPath(); err != nil {
			return rule, err
		}
	}
	if err := rule.indexMarkers(); err != nil {
		return rule, err
	}
	return rule, nil
}

func (rule compiledRule) validateFastPath() error {
	validLine := len(rule.lines) == 1 && rule.lines[0] == "//"
	validBlock := len(rule.blocks) == 1 && rule.blocks[0].open == "/*" && rule.blocks[0].close == "*/"
	if !validLine || !validBlock || len(rule.docs) != 0 {
		return fmt.Errorf("fast path requires exactly // and /* */ comment rules")
	}
	if rule.blocks[0].nested || rule.blocks[0].lineStart || rule.foldCase || rule.fortran {
		return fmt.Errorf("fast path does not support contextual or nested comments")
	}
	for _, quote := range rule.quotes {
		if strings.HasPrefix(quote.open, "/") {
			return fmt.Errorf("quote opener %q conflicts with the fast path", quote.open)
		}
		if quote.multiline && (rule.rawDelimiter == 0 || !strings.ContainsRune(quote.open, rule.rawDelimiter)) {
			return fmt.Errorf("multiline quote %q has no fast-path guard", quote.open)
		}
	}
	return nil
}

func ordinaryMultiline(name, open string) bool {
	if len(open) >= 3 || open == "`" {
		return true
	}
	switch name {
	case "C", "CHeader", "Cpp", "CppHeader", "CppModule", "Go", "Java", "JavaScript", "TypeScript",
		"D", "Rust", "Python", "Cython", "Mojo", "Bazel", "Snakemake", "CSharp", "Jsx", "Tsx", "ArkTS":
		return name == "Rust" && open == `"`
	default:
		// Like tokei, retain quote state across physical lines when no local
		// recovery policy is specified. This is not syntax validation.
		return true
	}
}

func doubledQuotes(name string) bool {
	switch name {
	case "Sql", "VisualBasic", "VB6", "VBScript", "Pascal", "FortranLegacy", "FortranModern", "Coq":
		return true
	default:
		return false
	}
}

func applyLocalRules(name string, rule *compiledRule) {
	switch name {
	case "C", "CHeader", "Cpp", "CppHeader", "CppModule", "Go", "Java":
		rule.quotes = append(rule.quotes, quoteRule{open: "'", close: "'", escape: '\\'})
		rule.fastSimple = true
	}
	switch name {
	case "C", "CHeader":
		rule.lineSplice = true
		rule.note += "; local character literals and line splicing"
	case "Cpp", "CppHeader", "CppModule":
		rule.raw = "rawCpp"
		rule.rawDelimiter = '"'
		rule.lineSplice = true
		rule.note += "; local dynamic raw strings, character literals and line splicing"
	case "Go":
		rule.quotes = append(rule.quotes, quoteRule{open: "`", close: "`", multiline: true})
		rule.rawDelimiter = '`'
		rule.note += "; local rune and raw-string rules"
	case "Java":
		rule.quotes = append(rule.quotes, quoteRule{open: `"""`, close: `"""`, escape: '\\', multiline: true})
		rule.rawDelimiter = '"'
		rule.note += "; local character literals and text blocks"
	case "Rust":
		rule.quotes = []quoteRule{
			{open: `"`, close: `"`, escape: '\\', multiline: true},
			{open: "'", close: "'", escape: '\\'},
		}
		rule.special = "syntaxRust"
		rule.raw = "rawRust"
		rule.note += "; local dynamic raw strings and lifetimes"
	case "JavaScript", "TypeScript":
		rule.special = "syntaxJavaScript"
		rule.note += "; local template interpolation and regex literals"
	case "Python", "Cython", "Mojo", "Bazel", "Snakemake":
		rule.special = "syntaxPython"
		rule.continuation = true
		rule.note += "; local statement-aware documentation"
	case "Assembly":
		// gocloc groups GAS and other assembler dialects under one name.
		rule.lines = []string{"//", ";", "#", "@", "|", "!"}
		rule.blocks = []blockRule{{open: "/*", close: "*/"}}
		rule.note += "; preserve combined assembler comment markers"
	case "FortranLegacy":
		rule.fortran = true
		rule.note += "; fixed-column comments"
	case "Batch", "Asp", "Autoit", "BrightScript", "VisualBasic", "VB6", "VBScript":
		rule.foldCase = true
	case "VimScript":
		// A double quote begins either a comment or an expression string in
		// Vimscript. Keep this ambiguous catalog entry but do not activate it.
		rule.note += "; ambiguous double-quote opener (activation deferred)"
		rule.ambiguous = true
	}
	if name == "Nim" {
		rule.blocks = append(rule.blocks, blockRule{open: "#[", close: "]#", nested: true})
		rule.note += "; local nested block comments"
	}
	if name == "Coq" || name == "FSharp" {
		for i := range rule.blocks {
			rule.blocks[i].nested = true
		}
		rule.note += "; local nested block comments"
	}
}

func (rule *compiledRule) indexMarkers() error {
	// Prefix overlaps are legal; the scanner chooses the longest match. An
	// identical opener with conflicting meanings is not silently accepted.
	meanings := map[string]string{}
	add := func(open, meaning string) error {
		if previous, exists := meanings[open]; exists && previous != meaning {
			vimQuote := rule.ambiguous && open == `"` && previous == "line comment"
			if vimQuote && strings.HasPrefix(meaning, "string ending") {
				return nil
			}
			return fmt.Errorf(
				"conflicting opening marker %q: %s and %s",
				open,
				previous,
				meaning,
			)
		}
		meanings[open] = meaning
		first := open[0]
		rule.starts[first>>6] |= uint64(1) << (first & 63)
		if rule.foldCase && first >= 'A' && first <= 'Z' {
			lower := first + ('a' - 'A')
			rule.starts[lower>>6] |= uint64(1) << (lower & 63)
		}
		return nil
	}
	for _, marker := range rule.lines {
		if err := add(marker, "line comment"); err != nil {
			return err
		}
	}
	for _, block := range rule.blocks {
		if err := add(block.open, fmt.Sprintf("block ending %q nested=%t", block.close, block.nested)); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		quotes []quoteRule
		kind   string
	}{{quotes: rule.quotes, kind: "string"}, {quotes: rule.docs, kind: "documentation"}} {
		for _, quote := range group.quotes {
			meaning := fmt.Sprintf(
				"%s ending %q escape=%d multiline=%t",
				group.kind,
				quote.close,
				quote.escape,
				quote.multiline,
			)
			if err := add(quote.open, meaning); err != nil {
				return err
			}
		}
	}
	return nil
}

func (rule compiledRule) write(out *bytes.Buffer) {
	fmt.Fprintf(
		out,
		"starts:[4]uint64{%d,%d,%d,%d},\nlineComments:[]string{",
		rule.starts[0],
		rule.starts[1],
		rule.starts[2],
		rule.starts[3],
	)
	for _, marker := range rule.lines {
		fmt.Fprintf(out, "%q,", marker)
	}
	out.WriteString("},\nblockComments:[]blockCommentRule{")
	for _, block := range rule.blocks {
		fmt.Fprintf(
			out,
			"{open:%q,close:%q,nested:%t,lineStart:%t},",
			block.open,
			block.close,
			block.nested,
			block.lineStart,
		)
	}
	out.WriteString("},\n")
	for _, group := range []struct {
		name   string
		quotes []quoteRule
	}{{name: "strings", quotes: rule.quotes}, {name: "docQuotes", quotes: rule.docs}} {
		fmt.Fprintf(out, "%s:[]stringRule{", group.name)
		for _, quote := range group.quotes {
			fmt.Fprintf(
				out,
				"{open:%q,close:%q,escape:%q,multiline:%t,doubled:%t},",
				quote.open,
				quote.close,
				quote.escape,
				quote.multiline,
				quote.doubled,
			)
		}
		out.WriteString("},\n")
	}
	fmt.Fprintf(
		out,
		"fastSimple:%t,rawDelimiter:%q,lineSplice:%t,continuation:%t,\n",
		rule.fastSimple,
		rule.rawDelimiter,
		rule.lineSplice,
		rule.continuation,
	)
	fmt.Fprintf(
		out,
		"special:%s,raw:%s,fortran:%t,foldCase:%t,\n",
		rule.special,
		rule.raw,
		rule.fortran,
		rule.foldCase,
	)
}
