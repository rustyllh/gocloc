package lexer

import (
	"bytes"
	"unicode/utf8"
)

// Raw delimiters belong to one file's scanner. They never borrow the reader's
// buffer, which is overwritten when the next physical line is read.
func (s *Scanner) beginRaw(line []byte, pos int) (int, bool) {
	if pos > 0 && identifierByte(line[pos-1]) {
		return pos, false
	}
	switch s.rules.raw {
	case rawCpp:
		for _, prefix := range []string{"R\"", "u8R\"", "uR\"", "UR\"", "LR\""} {
			if !bytes.HasPrefix(line[pos:], []byte(prefix)) {
				continue
			}
			start := pos + len(prefix)
			for end := start; end < len(line) && end-start <= len(s.rawDelimiter); end++ {
				ch := line[end]
				if ch == '(' {
					s.rawLength = copy(s.rawDelimiter[:], line[start:end])
					s.raw = true
					return end + 1, true
				}
				if ch <= ' ' || ch == ')' || ch == '\\' || ch >= 0x7f {
					break
				}
			}
			return pos, false
		}
	case rawRust:
		start := pos
		if line[start] == 'b' || line[start] == 'c' {
			start++
		}
		if start >= len(line) || line[start] != 'r' {
			return pos, false
		}
		start++
		end := start
		for end < len(line) && line[end] == '#' && end-start <= 255 {
			end++
		}
		if end < len(line) && line[end] == '"' && end-start <= 255 {
			s.rawHashes = end - start
			s.raw = true
			return end + 1, true
		}
	}
	return pos, false
}

func (s *Scanner) scanRaw(line []byte, pos int) int {
	if s.rules.raw == rawCpp {
		for pos < len(line) {
			offset := bytes.IndexByte(line[pos:], ')')
			if offset < 0 {
				break
			}
			pos += offset + 1
			end := pos + s.rawLength
			if end < len(line) && line[end] == '"' && bytes.Equal(line[pos:end], s.rawDelimiter[:s.rawLength]) {
				s.raw = false
				return end + 1
			}
		}
		return len(line)
	}
	for pos < len(line) {
		offset := bytes.IndexByte(line[pos:], '"')
		if offset < 0 {
			break
		}
		pos += offset + 1
		end := pos
		for end < len(line) && end-pos < s.rawHashes && line[end] == '#' {
			end++
		}
		if end-pos == s.rawHashes {
			s.raw = false
			return end
		}
	}
	return len(line)
}

func identifierByte(ch byte) bool {
	return isIdentifierPart(ch) || ch >= utf8.RuneSelf
}

func cppDigitSeparator(line []byte, pos int) bool {
	if pos == 0 || pos+1 >= len(line) {
		return false
	}
	if !identifierByte(line[pos-1]) || !identifierByte(line[pos+1]) {
		return false
	}
	start := pos - 1
	for start > 0 {
		ch := line[start-1]
		if !identifierByte(ch) && ch != '.' && ch != '\'' {
			break
		}
		start--
	}
	return line[start] >= '0' && line[start] <= '9'
}

// Rust lifetimes and labels start with an apostrophe but have no closing quote.
// Recognize complete character literals before entering the string state.
func rustCharacter(input []byte) bool {
	if len(input) < 3 {
		return false
	}
	end := 1
	if input[end] == '\\' {
		end++
		if end >= len(input) {
			return false
		}
		switch input[end] {
		case 'x':
			end += 3
		case 'u':
			end++
			if end >= len(input) || input[end] != '{' {
				return false
			}
			close := bytes.IndexByte(input[end:], '}')
			if close < 0 {
				return false
			}
			end += close + 1
		default:
			end++
		}
	} else {
		_, width := utf8.DecodeRune(input[end:])
		end += width
	}
	return end < len(input) && input[end] == '\''
}
