package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"slices"
	"strings"
)

const sourcePath = "languages.json"
const mappingPath = "mappings.json"
const outputPath = "../lexer/rules_generated.go"
const coveragePath = "COVERAGE.md"
const sourceSHA256 = "596748f92a5dca4065cc73e9379cf45615f1c06ba0b5b279b5851edd8258bd05"

type upstreamRule struct {
	Name           string     `json:"name"`
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
	Kind           string     `json:"kind"`
	Extensions     []string   `json:"extensions"`
	Filenames      []string   `json:"filenames"`
	Env            []string   `json:"env"`
	Shebangs       []string   `json:"shebangs"`
	PathSuffixes   []string   `json:"path_suffixes"`
	MIME           []string   `json:"mime"`
}

type upstreamRules struct {
	Languages map[string]upstreamRule `json:"languages"`
}

type languageMapping struct {
	Source     string `json:"source"`
	KeepLegacy string `json:"keep_legacy"`
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read rules: %w", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != sourceSHA256 {
		return fmt.Errorf("unexpected tokei rules SHA-256: %s", got)
	}
	var upstream upstreamRules
	if err := decodeJSON(content, &upstream); err != nil {
		return fmt.Errorf("decode rules: %w", err)
	}
	content, err = os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("read mappings: %w", err)
	}
	mappings := map[string]languageMapping{}
	if err := decodeJSON(content, &mappings); err != nil {
		return fmt.Errorf("decode mappings: %w", err)
	}
	code, report, err := generateArtifacts(upstream.Languages, mappings)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, code, 0644); err != nil {
		return fmt.Errorf("write generated rules: %w", err)
	}
	if err := os.WriteFile(coveragePath, report, 0644); err != nil {
		return fmt.Errorf("write coverage report: %w", err)
	}
	return nil
}

func decodeJSON(content []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected one JSON document, got trailing data: %v", err)
	}
	return nil
}

func generateArtifacts(
	upstream map[string]upstreamRule,
	mappings map[string]languageMapping,
) ([]byte, []byte, error) {
	if err := validateMappings(upstream, mappings); err != nil {
		return nil, nil, err
	}
	var out bytes.Buffer
	compiled := make(map[string]compiledRule, len(upstream))
	out.WriteString("// Code generated from internal/languagerules/languages.json and mappings.json; DO NOT EDIT.\n")
	out.WriteString("package lexer\n\nvar upstreamSyntax = map[string]*Rules{\n")
	for _, name := range sortedKeys(upstream) {
		rule, err := compileRule(name, upstream[name])
		if err != nil {
			return nil, nil, fmt.Errorf("compile %q: %w", name, err)
		}
		compiled[name] = rule
		fmt.Fprintf(&out, "// %s\n%q: {\n", rule.note, name)
		rule.write(&out)
		out.WriteString("},\n")
	}
	out.WriteString("}\n\nvar builtInSyntax = map[string]*Rules{\n")
	for _, name := range sortedKeys(mappings) {
		mapping := mappings[name]
		if mapping.KeepLegacy == "" {
			fmt.Fprintf(
				&out,
				"%q: upstreamSyntax[%q],\n",
				name,
				mapping.Source,
			)
		}
	}
	out.WriteString("}\n")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("format generated rules: %w", err)
	}
	return formatted, coverageReport(upstream, mappings, compiled), nil
}

func validateMappings(upstream map[string]upstreamRule, mappings map[string]languageMapping) error {
	for _, name := range sortedKeys(mappings) {
		mapping := mappings[name]
		if name == "" || mapping.Source == "" && mapping.KeepLegacy == "" {
			return fmt.Errorf("missing source or legacy reason for %q", name)
		}
		if mapping.Source == "" {
			continue
		}
		if _, exists := upstream[mapping.Source]; !exists {
			return fmt.Errorf("unknown source %q for %q", mapping.Source, name)
		}
		if deferredSource(mapping.Source) && mapping.KeepLegacy == "" {
			return fmt.Errorf("source %q needs an explicit legacy reason for %q", mapping.Source, name)
		}
	}
	return nil
}

func deferredSource(name string) bool {
	switch name {
	case "Html", "Vue", "Svelte", "RubyHtml", "GlimmerJs", "GlimmerTs", "Templ", "LinguaFranca",
		"Markdown", "Mdx", "Djot", "UnrealDeveloperMarkdown", "Text", "Jsx", "Tsx", "Jupyter", "VimScript":
		return true
	default:
		return false
	}
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func quotePair(pair []string) (string, string, error) {
	if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		return "", "", fmt.Errorf("invalid delimiter pair %q", pair)
	}
	return decodeMarker(pair[0]), decodeMarker(pair[1]), nil
}

// The snapshot escapes quotes for Rust code generation. Other backslashes are
// actual language delimiters (for example Q), not JSON escapes to unquote again.
func decodeMarker(marker string) string {
	return strings.ReplaceAll(marker, "\\\"", "\"")
}
