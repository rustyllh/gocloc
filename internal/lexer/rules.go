package lexer

import "slices"

// Rules is immutable after initialization. A scanner copies only its
// state, so definitions can be shared safely by concurrent file workers.
type Rules struct {
	lineComments  []string
	blockComments []blockCommentRule
	strings       []stringRule
	starts        [4]uint64
	// fastSimple is safe only for // and /* */ comment rules. Other rule sets
	// may have additional markers that a slash-only search would miss.
	fastSimple   bool
	rawDelimiter byte
	lineSplice   bool
	special      syntaxKind
}

type syntaxKind uint8

const (
	syntaxGeneric syntaxKind = iota
	syntaxJavaScript
)

type blockCommentRule struct {
	open, close string
	nested      bool
}

type stringRule struct {
	open, close string
	escape      byte
	multiline   bool
}

func BuiltIn(name string) *Rules {
	return builtInSyntax[name]
}

// MatchesLegacy preserves the historical corrected C/Go scanning behavior
// for caller-created definitions with exactly the built-in comment markers.
func (r *Rules) MatchesLegacy(lineComments []string, multiLines [][]string, hasRegex bool) bool {
	if r == nil || hasRegex || !slices.Equal(lineComments, r.lineComments) ||
		len(multiLines) != len(r.blockComments) {
		return false
	}
	for i, block := range multiLines {
		if len(block) != 2 || block[0] != r.blockComments[i].open ||
			block[1] != r.blockComments[i].close {
			return false
		}
	}
	return true
}
