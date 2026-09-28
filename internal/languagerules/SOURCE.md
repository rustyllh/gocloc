# Tokei language rules

Source: https://github.com/XAMPPRocky/tokei/blob/c14f744716272fadeb27a74443cdffa0af35f82f/languages.json

Upstream commit: `c14f744716272fadeb27a74443cdffa0af35f82f`

SHA-256 of `languages.json`: `596748f92a5dca4065cc73e9379cf45615f1c06ba0b5b279b5851edd8258bd05`

The snapshot is used to generate built-in lexical rules. `go generate ./...`
reads the checked-in file and does not access the network. The upstream work is
used under its MIT license; see `LICENCE-MIT` in this directory.

Current generated coverage: C, C Header, D, Go, JavaScript, and TypeScript.
C Header explicitly reuses C rules because tokei has no corresponding entry.
D's `/+ ... +/` comments use `nested_comments`; its ordinary `/* ... */`
comments do not nest. Go adds rune and raw-string rules missing from the
snapshot. C and C Header add backslash-newline continuation. JavaScript and
TypeScript use local template-interpolation and regex-literal state because
the snapshot only supplies their delimiters. The generator rejects unsupported
special lexical fields for selected languages instead of silently ignoring
them. File extensions and shebangs are deliberately not imported.

Other languages, including Python's `doc_quotes` and Rust's variable-length
raw-string delimiters, retain their existing scanner until their semantics are
implemented and compared separately. JavaScript's regex/division decision is
heuristic, not a full grammar parser; ambiguous syntax can still be classified
incorrectly.
