package lexer

import "testing"

func TestScannerNestedBlock(t *testing.T) {
	t.Parallel()
	rules := &Rules{
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: true}},
	}
	scanner := NewScanner(rules)
	comment := []byte("/* outer /* inner */ still outer */\n")
	if scanner.ScanLine(comment, comment) || scanner.InComment() {
		t.Fatal("nested block should end as one comment line")
	}
	code := []byte("code\n")
	if !scanner.ScanLine(code, code) || scanner.InComment() {
		t.Fatal("code after nested comment was not counted")
	}
}
