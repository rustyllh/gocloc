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
	// Zero means template text. Positive values count braces inside ${...}.
	templates []int
}

func NewScanner(rules *Rules) Scanner {
	return Scanner{
		rules:         rules,
		block:         -1,
		stringRule:    -1,
		expectOperand: true,
	}
}

func (s *Scanner) InComment() bool {
	return s.block >= 0
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
	if isCode, handled := s.fastLine(line, trimmed, continued); handled {
		return isCode
	}
	hasCode := s.stringRule >= 0 || s.inTemplateText() || s.regex
	for pos := 0; pos < len(line); {
		if s.lineComment {
			break
		}
		if s.block >= 0 {
			pos = s.scanBlock(line, pos)
			continue
		}
		if s.inTemplateText() {
			pos = s.scanTemplate(line, pos)
			hasCode = true
			continue
		}
		if s.stringRule >= 0 {
			pos = s.scanString(line, pos)
			hasCode = true
			continue
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
		if marker := s.lineCommentAt(line[pos:]); marker != "" {
			s.lineComment = true
			break
		}
		if block := s.blockAt(line[pos:]); block >= 0 {
			s.block = block
			s.depth = 1
			pos += len(s.rules.blockComments[block].open)
			continue
		}
		if s.rules.special == syntaxJavaScript && line[pos] == '`' {
			s.templates = append(s.templates, 0)
			hasCode = true
			pos++
			continue
		}
		if rule := s.stringAt(line[pos:]); rule >= 0 {
			s.stringRule = rule
			s.escaped = false
			hasCode = true
			pos += len(s.rules.strings[rule].open)
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
		if s.stringRule >= 0 && !s.rules.strings[s.stringRule].multiline {
			s.stringRule = -1
			s.escaped = false
		}
		s.regex = false // JavaScript regular expressions cannot span physical lines.
		s.regexClass = false
	}
	return hasCode
}

// Avoid per-byte interpretation for ordinary C/Go source lines. JavaScript
// needs token context even on lines without comments, so it takes the slow path.
func (s *Scanner) fastLine(line, trimmed []byte, continued bool) (bool, bool) {
	if s.rules.special != syntaxGeneric || continued || s.block >= 0 ||
		s.stringRule >= 0 || s.lineComment || !s.rules.fastSimple {
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

func (s *Scanner) lineCommentAt(input []byte) string {
	for _, marker := range s.rules.lineComments {
		if bytes.HasPrefix(input, []byte(marker)) {
			return marker
		}
	}
	return ""
}

func (s *Scanner) blockAt(input []byte) int {
	for i, rule := range s.rules.blockComments {
		if bytes.HasPrefix(input, []byte(rule.open)) {
			return i
		}
	}
	return -1
}

func (s *Scanner) stringAt(input []byte) int {
	for i, rule := range s.rules.strings {
		if bytes.HasPrefix(input, []byte(rule.open)) {
			return i
		}
	}
	return -1
}

func (s *Scanner) scanBlock(line []byte, pos int) int {
	rule := s.rules.blockComments[s.block]
	if !rule.nested {
		end := bytes.Index(line[pos:], []byte(rule.close))
		if end < 0 {
			return len(line)
		}
		s.block = -1
		s.depth = 0
		return pos + end + len(rule.close)
	}
	if bytes.HasPrefix(line[pos:], []byte(rule.close)) {
		s.depth--
		if s.depth == 0 {
			s.block = -1
		}
		return pos + len(rule.close)
	}
	if rule.nested && bytes.HasPrefix(line[pos:], []byte(rule.open)) {
		s.depth++
		return pos + len(rule.open)
	}
	return pos + 1
}

func (s *Scanner) scanString(line []byte, pos int) int {
	rule := s.rules.strings[s.stringRule]
	if rule.escape == 0 {
		end := bytes.Index(line[pos:], []byte(rule.close))
		if end < 0 {
			return len(line)
		}
		s.stringRule = -1
		s.expectOperand = false
		return pos + end + len(rule.close)
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
		s.stringRule = -1
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
