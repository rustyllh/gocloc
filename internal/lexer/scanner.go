package lexer

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

// Scanner is private to one file. The rules it references are read-only.
type Scanner struct {
	rules         *Rules
	block         int
	depth         int
	stringRule    int
	escaped       bool
	lineComment   bool
	regex         bool
	regexClass    bool
	expectOperand bool
	document      bool
	raw           bool
	rawDelimiter  [16]byte
	rawLength     int
	rawHashes     int
	python        pythonState
	blocks        []blockFrame
	// Zero means template text. Positive values count braces inside ${...}.
	templates []int
}

type blockFrame struct {
	rule, depth int
}

func NewScanner(rules *Rules) Scanner {
	return Scanner{
		rules:         rules,
		block:         -1,
		stringRule:    -1,
		expectOperand: true,
		python:        pythonState{moduleDocument: true},
	}
}

func (s *Scanner) InComment() bool {
	return s.block >= 0 || s.document
}

func (s *Scanner) inTemplateText() bool {
	return len(s.templates) > 0 && s.templates[len(s.templates)-1] == 0
}

func (s *Scanner) ScanLine(line, trimmed []byte) bool {
	hasNewline := len(line) > 0 && line[len(line)-1] == '\n'
	if hasNewline {
		line = bytes.TrimSuffix(line[:len(line)-1], []byte{'\r'})
	}
	continued := s.rules.lineSplice && hasNewline && bytes.HasSuffix(line, []byte{'\\'})
	if continued {
		line = line[:len(line)-1]
	}
	if s.rules.special == syntaxPython {
		s.python.beginLine(
			line,
			trimmed,
			s.stringRule >= 0,
			s.document,
		)
	}
	if isCode, handled := s.fastLine(line, trimmed, continued); handled {
		return isCode
	}
	hasCode := s.stringRule >= 0 && !s.document || s.inTemplateText() || s.regex || s.raw
	for pos := 0; pos < len(line); {
		if s.lineComment {
			break
		}
		if s.block >= 0 {
			pos = s.scanBlock(line, pos)
			continue
		}
		if s.raw {
			pos = s.scanRaw(line, pos)
			hasCode = true
			continue
		}
		if s.inTemplateText() {
			pos = s.scanTemplate(line, pos)
			hasCode = true
			continue
		}
		if s.stringRule >= 0 {
			isDocument := s.document
			pos = s.scanString(line, pos)
			if !isDocument {
				hasCode = true
			}
			continue
		}
		if s.rules.raw != rawNone {
			if end, ok := s.beginRaw(line, pos); ok {
				pos = end
				hasCode = true
				continue
			}
		}
		if s.rules.special == syntaxPython {
			if end, ok := s.beginPythonString(line, pos, hasCode); ok {
				pos = end
				if !s.document {
					hasCode = true
				}
				continue
			}
		}
		if s.regex {
			pos = s.scanRegex(line, pos)
			hasCode = true
			continue
		}
		if s.rules.special == syntaxJavaScript && len(s.templates) > 0 {
			last := len(s.templates) - 1
			switch line[pos] {
			case '{':
				s.templates[last]++
			case '}':
				s.templates[last]--
				if s.templates[last] == 0 {
					pos++
					hasCode = true
					continue
				}
			}
		}
		// Profiling comment-heavy C/Go input showed unnecessary quote searches
		// in longest matching. The generator proves these rule sets have only
		// // and /* */ comments and no quote opener starting with a slash.
		if s.rules.fastSimple && line[pos] == '/' && pos+1 < len(line) {
			switch line[pos+1] {
			case '/':
				s.lineComment = true
				pos = len(line)
				continue
			case '*':
				s.block, s.depth = 0, 1
				pos += 2
				continue
			}
		}
		// Generated rules carry a first-byte index. Hand-written rules without
		// one still take the complete marker checks below.
		if s.rules.special == syntaxGeneric && s.rules.starts != ([4]uint64{}) {
			ch := line[pos]
			if s.rules.starts[ch>>6]&(uint64(1)<<(ch&63)) == 0 {
				if ch < utf8.RuneSelf {
					if ch != ' ' && (ch < '\t' || ch > '\r') {
						hasCode = true
					}
					pos++
				} else {
					pos = s.scanCodeByte(line, pos, &hasCode)
				}
				continue
			}
		}
		marker := s.lineCommentAt(line, pos)
		block := s.blockAt(line, pos, false)
		blockLength := 0
		if block >= 0 {
			blockLength = len(s.rules.blockComments[block].open)
		}
		rule := s.stringAt(line[pos:])
		quoteLength := 0
		if rule >= 0 {
			quoteLength = len(s.rules.quote(rule).open)
		}
		// All token kinds participate in longest matching, not just quotes.
		// At equal length a block wins over a line comment, then a quote.
		if blockLength > 0 && blockLength >= len(marker) && blockLength >= quoteLength {
			s.block = block
			s.depth = 1
			pos += blockLength
			continue
		}
		if marker != "" && len(marker) >= quoteLength {
			s.lineComment = true
			break
		}
		if rule >= 0 {
			if s.rules.special == syntaxJavaScript && line[pos] == '`' {
				s.templates = append(s.templates, 0)
				hasCode = true
				pos++
				continue
			}
			if line[pos] == '\'' && s.rules.special == syntaxRust && !rustCharacter(line[pos:]) {
				pos = s.scanCodeByte(line, pos, &hasCode)
				continue
			}
			if line[pos] == '\'' && s.rules.raw == rawCpp && cppDigitSeparator(line, pos) {
				pos++
				hasCode = true
				continue
			}
			s.stringRule = rule
			s.escaped = false
			s.document = rule >= len(s.rules.strings) && !hasCode
			if !s.document {
				hasCode = true
			}
			pos += quoteLength
			continue
		}
		if s.rules.special == syntaxJavaScript && line[pos] == '/' {
			if s.expectOperand {
				s.regex = true
				s.escaped = false
				s.regexClass = false
			} else {
				s.expectOperand = true
			}
			hasCode = true
			pos++
			continue
		}
		pos = s.scanCodeByte(line, pos, &hasCode)
	}
	if !continued {
		s.lineComment = false
		stringContinues := s.rules.continuation && s.escaped && hasNewline
		if s.stringRule >= 0 && !s.rules.quote(s.stringRule).multiline && !stringContinues {
			s.stringRule = -1
			s.document = false
		}
		s.escaped = false
		s.regex = false // JavaScript regular expressions cannot span physical lines.
		s.regexClass = false
	}
	if s.rules.special == syntaxPython {
		s.python.finishLine(s.stringRule >= 0)
	}
	return hasCode
}

// Avoid per-byte interpretation for ordinary C/Go source lines. JavaScript
// needs token context even on lines without comments, so it takes the slow path.
func (s *Scanner) fastLine(line, trimmed []byte, continued bool) (bool, bool) {
	noMarkers := len(s.rules.lineComments)+len(s.rules.blockComments)+len(s.rules.strings)+len(s.rules.docQuotes) == 0
	if s.rules.special == syntaxGeneric && noMarkers {
		return len(trimmed) != 0, true
	}
	if s.rules.special != syntaxGeneric || continued || s.block >= 0 ||
		s.stringRule >= 0 || s.raw || s.lineComment || !s.rules.fastSimple {
		return false, false
	}
	if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("//")) {
		return false, true
	}
	for slash := bytes.IndexByte(line, '/'); slash >= 0; {
		if slash+1 < len(line) && (line[slash+1] == '/' || line[slash+1] == '*') {
			return false, false
		}
		next := bytes.IndexByte(line[slash+1:], '/')
		if next < 0 {
			break
		}
		slash += next + 1
	}
	if s.rules.rawDelimiter != 0 && bytes.IndexByte(line, s.rules.rawDelimiter) >= 0 {
		return false, false
	}
	return true, true
}

func (s *Scanner) lineCommentAt(line []byte, pos int) string {
	matched := ""
	for _, marker := range s.rules.lineComments {
		if s.rules.fortran && marker != "!" {
			if pos == 0 && bytes.HasPrefix(line, []byte(marker)) {
				return marker
			}
			continue
		}
		if len(marker) > len(matched) && s.markerAt(line, pos, marker) {
			matched = marker
		}
	}
	return matched
}

func (s *Scanner) blockAt(line []byte, pos int, nestedOnly bool) int {
	matched := -1
	for i, rule := range s.rules.blockComments {
		if nestedOnly && !rule.nested {
			continue
		}
		if rule.lineStart && pos != 0 {
			continue
		}
		if (matched < 0 || len(rule.open) > len(s.rules.blockComments[matched].open)) &&
			s.markerAt(line, pos, rule.open) {
			matched = i
		}
	}
	return matched
}

func (s *Scanner) markerAt(line []byte, pos int, marker string) bool {
	end := pos + len(marker)
	if end > len(line) {
		return false
	}
	matched := bytes.Equal(line[pos:end], []byte(marker))
	if s.rules.foldCase {
		matched = bytes.EqualFold(line[pos:end], []byte(marker))
	}
	if !matched {
		return false
	}
	// Word-like markers (REM, dnl, =pod) must not consume identifiers.
	first, last := marker[0], marker[len(marker)-1]
	if isIdentifierPart(first) && pos > 0 && identifierByte(line[pos-1]) {
		return false
	}
	if isIdentifierPart(last) && end < len(line) && identifierByte(line[end]) {
		return false
	}
	return true
}

func (s *Scanner) stringAt(input []byte) int {
	matched := -1
	for i := range len(s.rules.strings) + len(s.rules.docQuotes) {
		rule := s.rules.quote(i)
		if bytes.HasPrefix(input, []byte(rule.open)) &&
			(matched < 0 || len(rule.open) > len(s.rules.quote(matched).open)) {
			matched = i
		}
	}
	return matched
}

func (s *Scanner) scanBlock(line []byte, pos int) int {
	rule := s.rules.blockComments[s.block]
	if rule.lineStart {
		if pos == 0 && s.markerAt(line, pos, rule.close) {
			s.closeBlock()
			return len(rule.close)
		}
		return len(line)
	}
	if !rule.nested {
		if s.rules.foldCase {
			for end := pos; end+len(rule.close) <= len(line); end++ {
				if s.markerAt(line, end, rule.close) {
					s.closeBlock()
					return end + len(rule.close)
				}
			}
			return len(line)
		}
		end := bytes.Index(line[pos:], []byte(rule.close))
		if end < 0 {
			return len(line)
		}
		s.closeBlock()
		return pos + end + len(rule.close)
	}
	nested := s.blockAt(line, pos, true)
	// Inside a nested block only nested openers compete with its closer.
	// Prefer the longest delimiter, with the closer winning equal lengths.
	closeWins := nested < 0 || len(rule.close) >= len(s.rules.blockComments[nested].open)
	if closeWins && s.markerAt(line, pos, rule.close) {
		s.closeBlock()
		return pos + len(rule.close)
	}
	if nested >= 0 {
		if nested == s.block {
			s.depth++
		} else {
			s.blocks = append(s.blocks, blockFrame{rule: s.block, depth: s.depth})
			s.block = nested
			s.depth = 1
		}
		return pos + len(s.rules.blockComments[nested].open)
	}
	return pos + 1
}

func (s *Scanner) closeBlock() {
	s.depth--
	if s.depth > 0 {
		return
	}
	if len(s.blocks) > 0 {
		last := s.blocks[len(s.blocks)-1]
		s.blocks = s.blocks[:len(s.blocks)-1]
		s.block, s.depth = last.rule, last.depth
		return
	}
	s.block = -1
}

func (s *Scanner) scanString(line []byte, pos int) int {
	rule := s.rules.quote(s.stringRule)
	if rule.escape == 0 {
		end := bytes.Index(line[pos:], []byte(rule.close))
		if end < 0 {
			return len(line)
		}
		end += pos
		if rule.doubled && bytes.HasPrefix(line[end+len(rule.close):], []byte(rule.close)) {
			return end + 2*len(rule.close)
		}
		s.stringRule = -1
		s.document = false
		s.expectOperand = false
		return end + len(rule.close)
	}
	if s.escaped {
		s.escaped = false
		return pos + 1
	}
	if rule.escape != 0 && line[pos] == rule.escape {
		s.escaped = true
		return pos + 1
	}
	if bytes.HasPrefix(line[pos:], []byte(rule.close)) {
		if rule.doubled && bytes.HasPrefix(line[pos+len(rule.close):], []byte(rule.close)) {
			return pos + 2*len(rule.close)
		}
		s.stringRule = -1
		s.document = false
		s.expectOperand = false
		return pos + len(rule.close)
	}
	return pos + 1
}

func (s *Scanner) scanTemplate(line []byte, pos int) int {
	if s.escaped {
		s.escaped = false
		return pos + 1
	}
	if line[pos] == '\\' {
		s.escaped = true
		return pos + 1
	}
	if line[pos] == '`' {
		s.templates = s.templates[:len(s.templates)-1]
		s.expectOperand = false
		return pos + 1
	}
	if bytes.HasPrefix(line[pos:], []byte("${")) {
		s.templates[len(s.templates)-1] = 1
		s.expectOperand = true
		return pos + 2
	}
	return pos + 1
}

func (s *Scanner) scanRegex(line []byte, pos int) int {
	if s.escaped {
		s.escaped = false
		return pos + 1
	}
	switch line[pos] {
	case '\\':
		s.escaped = true
	case '[':
		s.regexClass = true
	case ']':
		s.regexClass = false
	case '/':
		if !s.regexClass {
			s.regex = false
			s.expectOperand = false
		}
	}
	return pos + 1
}

func (s *Scanner) scanCodeByte(line []byte, pos int, hasCode *bool) int {
	ch := line[pos]
	if s.rules.special == syntaxPython {
		s.python.codeByte(ch)
	}
	if s.rules.special == syntaxJavaScript && isIdentifierStart(ch) {
		end := pos + 1
		for end < len(line) && isIdentifierPart(line[end]) {
			end++
		}
		word := line[pos:end]
		s.expectOperand = bytes.Equal(word, []byte("return")) ||
			bytes.Equal(word, []byte("throw")) || bytes.Equal(word, []byte("case")) ||
			bytes.Equal(word, []byte("yield")) || bytes.Equal(word, []byte("await"))
		*hasCode = true
		return end
	}
	if ch >= '0' && ch <= '9' {
		s.expectOperand = false
		*hasCode = true
		return pos + 1
	}
	r, width := rune(ch), 1
	if ch >= utf8.RuneSelf {
		r, width = utf8.DecodeRune(line[pos:])
	}
	if !unicode.IsSpace(r) {
		*hasCode = true
		switch ch {
		case ')', ']', '}':
			s.expectOperand = false
		case '.', '+', '-', '=', ':', ',', ';', '(', '[', '{', '!', '?', '*', '&', '|':
			s.expectOperand = true
		}
	}
	return pos + width
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || ch == '$' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func isIdentifierPart(ch byte) bool {
	return isIdentifierStart(ch) || ch >= '0' && ch <= '9'
}
