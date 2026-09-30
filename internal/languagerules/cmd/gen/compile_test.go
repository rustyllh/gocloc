package main

import "testing"

func TestCompileRule(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		rule  upstreamRule
		valid bool
	}{
		{name: "no comments is valid", valid: true},
		{name: "metadata is not syntax", rule: upstreamRule{Blank: true, Literate: true, Kind: "html"}, valid: true},
		{name: "nested blocks", rule: upstreamRule{Nested: true, BlockComments: [][]string{{"(*", "*)"}}}, valid: true},
		{name: "line-block overlap", valid: true, rule: upstreamRule{
			LineComments: []string{"#"}, BlockComments: [][]string{{"###", "###"}},
		}},
		{name: "ordinary-doc overlap", valid: true, rule: upstreamRule{
			Quotes: [][]string{{"'", "'"}}, DocQuotes: [][]string{{"'''", "'''"}},
		}},
		{name: "ordinary-verbatim overlap", valid: true, rule: upstreamRule{
			Quotes: [][]string{{"'", "'"}}, VerbatimQuotes: [][]string{{"'''", "'''"}},
		}},
		{name: "empty line marker", rule: upstreamRule{LineComments: []string{""}}},
		{name: "missing block closer", rule: upstreamRule{BlockComments: [][]string{{"open"}}}},
		{name: "missing raw closer", rule: upstreamRule{VerbatimQuotes: [][]string{{"open"}}}},
		{name: "identical nested markers", rule: upstreamRule{Nested: true, BlockComments: [][]string{{"x", "x"}}}},
		{name: "same opener different meaning", rule: upstreamRule{
			LineComments: []string{"'"}, Quotes: [][]string{{"'", "'"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileRule("test", test.rule)
			if (err == nil) != test.valid {
				t.Fatalf("compile = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestCompileFastPathValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		rule  upstreamRule
		valid bool
	}{
		{name: "valid C", valid: true, rule: upstreamRule{
			LineComments: []string{"//"}, BlockComments: [][]string{{"/*", "*/"}}, Quotes: [][]string{{"\"", "\""}},
		}},
		{name: "extra line marker", rule: upstreamRule{
			LineComments: []string{"//", "#"}, BlockComments: [][]string{{"/*", "*/"}},
		}},
		{name: "nested comment", rule: upstreamRule{
			LineComments: []string{"//"}, BlockComments: [][]string{{"/*", "*/"}}, Nested: true,
		}},
		{name: "slash quote opener", rule: upstreamRule{
			LineComments: []string{"//"}, BlockComments: [][]string{{"/*", "*/"}}, Quotes: [][]string{{"/quote", "quote/"}},
		}},
		{name: "unguarded multiline quote", rule: upstreamRule{
			LineComments: []string{"//"}, BlockComments: [][]string{{"/*", "*/"}}, Quotes: [][]string{{"'''", "'''"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileRule("C", test.rule)
			if (err == nil) != test.valid {
				t.Fatalf("fast path validation = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestCompileQuoteSemantics(t *testing.T) {
	t.Parallel()
	rule, err := compileRule("CSharp", upstreamRule{
		Quotes: [][]string{{"\"", "\""}}, VerbatimQuotes: [][]string{{"@\"", "\""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.quotes) != 2 || rule.quotes[0].escape != '\\' || rule.quotes[0].multiline ||
		rule.quotes[1].escape != 0 || !rule.quotes[1].multiline || !rule.quotes[1].doubled {
		t.Fatalf("ordinary and verbatim quotes lost their distinct semantics: %+v", rule.quotes)
	}
	docs, err := compileRule("Pony", upstreamRule{DocQuotes: [][]string{{"\"\"\"", "\"\"\""}}})
	if err != nil || len(docs.docs) != 1 || len(docs.quotes) != 0 {
		t.Fatalf("documentation delimiters were merged with ordinary strings: %+v/%v", docs, err)
	}
}
