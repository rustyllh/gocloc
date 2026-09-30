package lexer

import "bytes"

type pythonSuite struct {
	indent   int
	document bool
}

// This tracks statement starts and def/class suites, not Python's full grammar.
// Only their first standalone string is counted as documentation; assigned
// triple-quoted strings, bytes and formatted strings remain code.
type pythonState struct {
	suites         []pythonSuite
	moduleDocument bool
	candidate      bool
	indent         int
	brackets       int
	header         bool
	headerIndent   int
	pending        bool
	pendingIndent  int
	continued      bool
	last           byte
}

func (p *pythonState) beginLine(line, trimmed []byte, inString, document bool) {
	p.candidate = document
	p.last = 0
	if inString || len(trimmed) == 0 || trimmed[0] == '#' {
		return
	}
	if p.brackets > 0 || p.continued {
		p.continued = false
		return
	}
	p.indent = 0
	for _, ch := range line {
		if ch == ' ' {
			p.indent++
			continue
		}
		if ch == '\t' {
			p.indent += 8 - p.indent%8
			continue
		}
		break
	}
	for len(p.suites) > 0 && p.indent < p.suites[len(p.suites)-1].indent {
		p.suites = p.suites[:len(p.suites)-1]
	}
	if p.pending {
		if p.indent > p.pendingIndent {
			p.suites = append(p.suites, pythonSuite{indent: p.indent, document: true})
		}
		p.pending = false
	}
	if len(p.suites) == 0 {
		p.candidate = p.moduleDocument && p.indent == 0
		p.moduleDocument = false
	} else {
		current := &p.suites[len(p.suites)-1]
		p.candidate = current.document && p.indent == current.indent
		current.document = false
	}
	if bytes.HasPrefix(trimmed, []byte("def ")) || bytes.HasPrefix(trimmed, []byte("class ")) ||
		bytes.HasPrefix(trimmed, []byte("async def ")) {
		p.header = true
		p.headerIndent = p.indent
	}
}

func (p *pythonState) codeByte(ch byte) {
	switch ch {
	case '(', '[', '{':
		p.brackets++
	case ')', ']', '}':
		if p.brackets > 0 {
			p.brackets--
		}
	}
	if ch != ' ' && (ch < '\t' || ch > '\r') {
		p.last = ch
		p.candidate = false
	}
}

func (p *pythonState) finishLine(inString bool) {
	p.continued = !inString && p.last == '\\'
	if p.header && !inString && p.brackets == 0 && !p.continued {
		if p.last == ':' {
			p.pending = true
			p.pendingIndent = p.headerIndent
		}
		p.header = false
	}
}

func (s *Scanner) beginPythonString(line []byte, pos int, hasCode bool) (int, bool) {
	start := pos
	docEligible := true
	if line[pos] != '\'' && line[pos] != '"' {
		if pos > 0 && identifierByte(line[pos-1]) {
			return pos, false
		}
		for pos < len(line) && pos-start < 2 {
			ch := line[pos] | 0x20
			if ch != 'r' && ch != 'u' && ch != 'b' && ch != 'f' {
				break
			}
			if ch == 'b' || ch == 'f' {
				docEligible = false
			}
			pos++
		}
		if pos == start || pos >= len(line) || line[pos] != '\'' && line[pos] != '"' {
			return start, false
		}
	}
	rule := s.stringAt(line[pos:])
	if rule < 0 {
		return start, false
	}
	s.stringRule = rule
	s.escaped = false
	s.document = s.python.candidate && !hasCode && docEligible
	s.python.candidate = s.document
	s.python.last = 's'
	return pos + len(s.rules.quote(rule).open), true
}
