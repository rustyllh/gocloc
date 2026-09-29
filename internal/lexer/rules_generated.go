// Code generated from internal/languagerules/languages.json; DO NOT EDIT.
package lexer

var builtInSyntax = map[string]*Rules{
	// C: tokei C; local character-literal and line-splice rules
	"C": {
		starts:        [4]uint64{141304424038400, 0, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}},
		fastSimple:    true,
		lineSplice:    true,
	},
	// C Header: tokei C (explicit name mapping); local character-literal and line-splice rules
	"C Header": {
		starts:        [4]uint64{141304424038400, 0, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}},
		fastSimple:    true,
		lineSplice:    true,
	},
	// D: tokei D
	"D": {
		starts:        [4]uint64{141304424038400, 0, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}, {open: "/+", close: "+/", nested: true}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}},
	},
	// Go: tokei Go; local rune and raw-string rules
	"Go": {
		starts:        [4]uint64{141304424038400, 4294967296, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}, {open: "`", close: "`", multiline: true}},
		fastSimple:    true,
		rawDelimiter:  '`',
	},
	// JavaScript: tokei JavaScript; local template and regex-literal rules
	"JavaScript": {
		starts:        [4]uint64{141304424038400, 4294967296, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}, {open: "`", close: "`", escape: '\\', multiline: true}},
		special:       syntaxJavaScript,
	},
	// TypeScript: tokei TypeScript; local template and regex-literal rules
	"TypeScript": {
		starts:        [4]uint64{141304424038400, 4294967296, 0, 0},
		lineComments:  []string{"//"},
		blockComments: []blockCommentRule{{open: "/*", close: "*/", nested: false}},
		strings:       []stringRule{{open: "\"", close: "\"", escape: '\\'}, {open: "'", close: "'", escape: '\\'}, {open: "`", close: "`", escape: '\\', multiline: true}},
		special:       syntaxJavaScript,
	},
}
