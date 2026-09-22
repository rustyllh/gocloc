package gocloc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// Frozen string-based parser from 136a87b. Keep this test oracle independent of
// the byte parser so differential tests catch changes to counting and callbacks.
// Only diagnostic formatting is shared; exact records are tested separately.
func referenceAnalyzeReader(filename string, language *Language, file io.Reader, opts *ClocOptions) (*ClocFile, error) {
	if opts.Debug {
		opts.diagnosticf("[FILE] file=%q\n", filename)
	}

	clocFile := &ClocFile{
		Name: filename,
		Lang: language.Name,
	}

	isFirstLine := true
	var inComments [][2]string
	reader := bufio.NewReader(file)

scannerloop:
	for {
		lineOrg, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return clocFile, fmt.Errorf("read %q: %w", filename, err)
		}

		// prevent infinite loop
		if len(lineOrg) == 0 && err == io.EOF {
			break
		}

		line := strings.TrimSpace(lineOrg)

		if len(line) == 0 {
			referenceOnBlank(clocFile, opts, len(inComments) > 0, line, lineOrg)
			continue
		}

		// shebang line is 'code'
		if isFirstLine && strings.HasPrefix(line, "#!") {
			referenceOnCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
			isFirstLine = false
			continue
		}

		if len(inComments) == 0 {
			if isFirstLine {
				line = trimBOM(line)
			}

			if len(language.regexLineComments) > 0 {
			singleloopRegex:
				for _, singleCommentRegex := range language.regexLineComments {
					if singleCommentRegex.MatchString(line) {
						// check if single comment is a prefix of multi comment
						for _, ml := range language.multiLines {
							if ml[0] != "" && strings.HasPrefix(line, ml[0]) {
								break singleloopRegex
							}
						}
						referenceOnComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
						continue scannerloop
					}
				}
			} else {
			singleloop:
				for _, singleComment := range language.lineComments {
					if strings.HasPrefix(line, singleComment) {
						// check if single comment is a prefix of multi comment
						for _, ml := range language.multiLines {
							if ml[0] != "" && strings.HasPrefix(line, ml[0]) {
								break singleloop
							}
						}
						referenceOnComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
						continue scannerloop
					}
				}
			}

			if len(language.multiLines) == 0 {
				referenceOnCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
				continue scannerloop
			}
		}

		if len(inComments) == 0 && !containsComment(line, language.multiLines) {
			referenceOnCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
			continue scannerloop
		}

		lenLine := len(line)
		if len(language.multiLines) == 1 && len(language.multiLines[0]) == 2 && language.multiLines[0][0] == "" {
			referenceOnCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
			continue
		}
		codeFlags := make([]bool, len(language.multiLines))
		for pos := 0; pos < lenLine; {
			for idx, ml := range language.multiLines {
				begin, end := ml[0], ml[1]
				lenBegin := len(begin)

				if pos+lenBegin <= lenLine && strings.HasPrefix(line[pos:], begin) && (begin != end || len(inComments) == 0) {
					pos += lenBegin
					inComments = append(inComments, [2]string{begin, end})
					continue
				}

				if n := len(inComments); n > 0 {
					last := inComments[n-1]
					if pos+len(last[1]) <= lenLine && strings.HasPrefix(line[pos:], last[1]) {
						inComments = inComments[:n-1]
						pos += len(last[1])
					}
				} else if pos < lenLine && !unicode.IsSpace(nextRune(line[pos:])) {
					codeFlags[idx] = true
				}
			}
			pos++
		}

		isCode := true
		for _, b := range codeFlags {
			if !b {
				isCode = false
			}
		}

		if isCode {
			referenceOnCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
		} else {
			referenceOnComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
		}

		if err == io.EOF {
			break
		}
	}

	return clocFile, nil
}

func referenceOnBlank(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Blanks++
	if opts.OnBlank != nil {
		opts.OnBlank(line)
	}

	if opts.Debug {
		opts.debugLine(
			"BLNK",
			clocFile,
			isInComments,
			[]byte(lineOrg),
		)
	}
}

func referenceOnComment(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Comments++
	if opts.OnComment != nil {
		opts.OnComment(line)
	}

	if opts.Debug {
		opts.debugLine(
			"COMM",
			clocFile,
			isInComments,
			[]byte(lineOrg),
		)
	}
}

func referenceOnCode(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Code++
	if opts.OnCode != nil {
		opts.OnCode(line)
	}

	if opts.Debug {
		opts.debugLine(
			"CODE",
			clocFile,
			isInComments,
			[]byte(lineOrg),
		)
	}
}
