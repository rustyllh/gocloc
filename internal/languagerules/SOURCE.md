# Tokei language rules

Source: https://github.com/XAMPPRocky/tokei/blob/c14f744716272fadeb27a74443cdffa0af35f82f/languages.json

Upstream commit: `c14f744716272fadeb27a74443cdffa0af35f82f`

SHA-256 of `languages.json`: `596748f92a5dca4065cc73e9379cf45615f1c06ba0b5b279b5851edd8258bd05`

The snapshot is used under its MIT license; see LICENCE-MIT in this directory.

## Generation and activation

Run `go generate ./...` to regenerate rules_generated.go and COVERAGE.md. The
generator reads the checked-in snapshot and mappings.json without network access.
It generates all **333** upstream entries, including languages with no comment
delimiters. Strict JSON decoding rejects unknown fields, malformed delimiters and
conflicting opener meanings; prefix overlaps are resolved by longest matching.
Vimscript's ambiguous double quote is an explicitly deferred exception.

mappings.json lists all **188** existing gocloc names explicitly. **138** enable
the generated scanner; **50** retain the legacy scanner, either because the
snapshot has no corresponding language or because activation needs unsupported
context. See [COVERAGE.md](COVERAGE.md) for the exact mappings and local additions.
The generated catalog is larger than the set of languages enabled in the CLI.

This migration does **not** import extensions, filenames, shebangs, environment
names, path suffixes or MIME types. It does not add file types, rename results or
change file detection. Caller-created NewLanguage definitions still use their
own markers and regexes. The historical corrected C/Go scanner remains available
when custom definitions have exactly the built-in comment markers. Explicit
regex overrides on built-in definitions also retain legacy semantics.

HTML, Vue, Svelte, Ruby HTML, Templ, Markdown, JSX, notebooks and other embedding
or literate contexts are not activated by this change. Plain Text continues
counting nonblank text as code. Their data is generated, but script/style blocks,
code fences, notebook cells and prose classification are deliberately deferred.
The upstream blank flag is metadata, not an instruction to classify nonblank
lines as blank. Literate, kind and important_syntax metadata is recorded in the
coverage report, not silently treated as generic lexical syntax.

## Generic scanning semantics

- Line and block comments, ordinary quotes, verbatim quotes and doc quotes all
  participate in longest opening-delimiter matching. Equal-length block/line
  collisions prefer the block; other conflicting meanings fail generation.
- Ordinary quotes recognize backslash escapes. Verbatim quotes do not. C#/F#
  verbatim quotes and selected SQL/VB/Pascal-style quotes recognize doubled
  closers. Triple quotes and verbatim quotes can span physical lines.
- The snapshot does not describe newline recovery. Like tokei, generic ordinary
  quotes retain state across lines unless there is an explicit local recovery
  policy (such as C, Go, Java, JavaScript and Python). Malformed source is
  classified, not rejected or reparsed.
- Nested blocks recognize their matching closer and supported nested openers,
  including distinct nested block forms. Non-nested blocks close at the first
  matching closer. Comment markers inside quotes never start comments.
- Doc delimiters remain separate from ordinary quotes. Generic standalone doc
  strings count as documentation; assigned strings count as code. Raku POD and
  inline documentation forms are comments. This is not AST-level classification.
- Word-like comment markers such as REM and dnl respect identifier boundaries.
  Selected BASIC/batch rules are case-insensitive. Ruby/Perl blocks are column
  anchored; fixed-form Fortran comment letters are recognized only in column 1.

Physically blank lines count as blank even inside comments and strings. Any
nonblank line with code outside comments counts as code, including code mixed
with comments. Counts, callbacks, debug and fragmented reads share the same
per-file scanner. Rules are immutable and shared; scanning state is never shared
between concurrent files.

## Local supplements and limits

C/C Header add character literals and backslash-newline splicing. C++ variants
add dynamic raw delimiters (up to 16 characters), encoded prefixes, character
literals and numeric digit separators. Go adds runes and raw strings. Java adds
character literals and text blocks. Rust adds raw strings with 0-255 hashes,
byte/C raw prefixes and lifetime/label distinctions. Assembly retains gocloc's
combined dialect comment markers. Nim, Coq and F# add missing nested-block rules.

Python uses a lightweight statement/indentation model: the first standalone
string at module level or in a def/class suite is documentation. Preambles,
shebangs and blanks do not consume that position. Assigned triples, later
expressions, bytes and f-strings are code. The same strategy covers Python-like
Cython, Mojo, Bazel/Starlark and Snakemake declarations; language-specific
declaration extensions are not a full grammar. Parenthesized docstrings,
operations applied after a multiline docstring closes and f-string replacement
expressions are not fully parsed.

JavaScript/TypeScript retain local template interpolation and heuristic
regex/division handling. A generated delimiter table does not supply dynamic
heredocs, arbitrary language interpolation, macros or other grammar-dependent
features not listed here. Full catalog coverage is not a claim of complete
parsing or identical output to tokei.

## Verification

internal/lexertest holds independent per-line expectations. Regression tests
cover normalized callback contents/order, debug, fragmented reads, malformed
sources, longest matching, raw quotes, nesting and documentation. Generation
tests verify checksum, complete name mappings, deterministic artifacts and
explicit deferrals. Fuzzing checks per-file state and physical-line conservation.

The benchmark script measures full analysis, workers 1/8 and both dedup modes
using versioned multilingual fixtures. See [scripts/BENCHMARKS.md](../../scripts/BENCHMARKS.md).
