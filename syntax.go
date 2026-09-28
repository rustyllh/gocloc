package gocloc

import "slices"

// syntaxRules is immutable after initialization. A scanner copies only its
// state, so definitions can be shared safely by concurrent file workers.
type syntaxRules struct {
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

func syntaxForLanguage(language *Language) *syntaxRules {
	if language.syntax != nil {
		return language.syntax
	}
	// Historically NewLanguage("Go"/"C"/"C Header", ...) received the
	// corrected lexer when its comment rules exactly matched the built-ins.
	// Preserve that behavior without selecting a scanner from the name alone.
	switch language.Name {
	case "Go", "C", "C Header":
	default:
		return nil
	}
	rules := builtInSyntax[language.Name]
	if len(language.regexLineComments) != 0 ||
		!slices.Equal(language.lineComments, rules.lineComments) ||
		len(language.multiLines) != len(rules.blockComments) {
		return nil
	}
	for i, block := range language.multiLines {
		if len(block) != 2 || block[0] != rules.blockComments[i].open ||
			block[1] != rules.blockComments[i].close {
			return nil
		}
	}
	return rules
}
