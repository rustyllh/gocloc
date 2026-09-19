package gocloc

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
)

// ClocFile is collecting to line count result.
type ClocFile struct {
	Code     int32  `xml:"code,attr" json:"code"`
	Comments int32  `xml:"comment,attr" json:"comment"`
	Blanks   int32  `xml:"blank,attr" json:"blank"`
	Name     string `xml:"name,attr" json:"name"`
	Lang     string `xml:"language,attr" json:"language"`
}

// ClocFiles is gocloc result set.
type ClocFiles []ClocFile

func (cf ClocFiles) SortByName() {
	sortFunc := func(i, j int) bool {
		return cf[i].Name < cf[j].Name
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByComments() {
	sortFunc := func(i, j int) bool {
		if cf[i].Comments == cf[j].Comments {
			return cf[i].Code > cf[j].Code
		}
		return cf[i].Comments > cf[j].Comments
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByBlanks() {
	sortFunc := func(i, j int) bool {
		if cf[i].Blanks == cf[j].Blanks {
			return cf[i].Code > cf[j].Code
		}
		return cf[i].Blanks > cf[j].Blanks
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByCode() {
	sortFunc := func(i, j int) bool {
		return cf[i].Code > cf[j].Code
	}
	sort.Slice(cf, sortFunc)
}

// AnalyzeFile analyzes a file and reports failures through opts.Diagnostics.
// Its signature is retained for callers; Processor excludes failed files.
func AnalyzeFile(filename string, language *Language, opts *ClocOptions) *ClocFile {
	opts = opts.withDiagnostics()
	file, err := analyzeFile(filename, language, opts)
	opts.warn(err)
	if file == nil {
		return &ClocFile{Name: filename}
	}
	return file
}

func analyzeFile(filename string, language *Language, opts *ClocOptions) (*ClocFile, error) {
	fp, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	file, readErr := analyzeReader(filename, language, fp, opts)
	return file, errors.Join(readErr, fp.Close())
}

type hashingReader struct {
	reader io.Reader
	hash   hash.Hash
}

// detectAndAnalyzeFile reuses the detection prefix for line analysis and, when
// deduplication is enabled, hashes physical reads once. Workers bound open files.
func detectAndAnalyzeFile(candidate fileCandidate, languages *DefinedLanguages, opts *ClocOptions) (result fileAnalysisResult) {
	result = fileAnalysisResult{fileCandidate: candidate, ignored: true}
	file, err := os.Open(candidate.path)
	if err != nil {
		result.err = err
		return result
	}
	defer func() {
		result.err = errors.Join(result.err, file.Close())
	}()

	var source io.Reader = file
	var hashed *hashingReader
	if !opts.SkipDuplicated {
		hashed = &hashingReader{reader: file, hash: md5.New()}
		source = hashed
	}
	reader := bufio.NewReader(source)
	var prefix []byte
	var detectionErr error
	ext, ok := detectFileType(candidate.path, opts, func(all bool) ([]byte, error) {
		var readErr error
		if all {
			prefix, readErr = io.ReadAll(reader)
		} else {
			prefix, readErr = reader.ReadBytes('\n')
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			detectionErr = fmt.Errorf("read %q for language detection: %w", candidate.path, readErr)
		}
		return prefix, readErr
	})
	if detectionErr != nil {
		result.err = detectionErr
		return result
	}
	if !ok {
		return result
	}
	languageKey, ok := Exts[ext]
	if !ok {
		return result
	}
	if !includeLanguage(languageKey, opts) {
		return result
	}
	result.languageKey = languageKey
	content := io.MultiReader(bytes.NewReader(prefix), reader)
	result.clocFile, result.err = analyzeReader(candidate.path, languages.Langs[languageKey], content, opts)
	if result.err != nil {
		return result
	}
	if hashed != nil {
		copy(result.digest[:], hashed.hash.Sum(nil))
	}
	result.ignored = false
	return result
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		if _, hashErr := r.hash.Write(p[:n]); hashErr != nil {
			return n, hashErr
		}
	}
	return n, err
}

// AnalyzeReader counts lines and reports read failures through opts.Diagnostics.
// Partial counts are returned for compatibility; Processor excludes failed files.
func AnalyzeReader(filename string, language *Language, file io.Reader, opts *ClocOptions) *ClocFile {
	opts = opts.withDiagnostics()
	result, err := analyzeReader(filename, language, file, opts)
	opts.warn(err)
	return result
}

func analyzeReader(filename string, language *Language, file io.Reader, opts *ClocOptions) (*ClocFile, error) {
	if opts.Debug {
		opts.diagnosticf("filename=%v\n", filename)
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
			onBlank(clocFile, opts, len(inComments) > 0, line, lineOrg)
			continue
		}

		// shebang line is 'code'
		if isFirstLine && strings.HasPrefix(line, "#!") {
			onCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
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
						onComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
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
						onComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
						continue scannerloop
					}
				}
			}

			if len(language.multiLines) == 0 {
				onCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
				continue scannerloop
			}
		}

		if len(inComments) == 0 && !containsComment(line, language.multiLines) {
			onCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
			continue scannerloop
		}

		lenLine := len(line)
		if len(language.multiLines) == 1 && len(language.multiLines[0]) == 2 && language.multiLines[0][0] == "" {
			onCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
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
			onCode(clocFile, opts, len(inComments) > 0, line, lineOrg)
		} else {
			onComment(clocFile, opts, len(inComments) > 0, line, lineOrg)
		}

		if err == io.EOF {
			break
		}
	}

	return clocFile, nil
}

func onBlank(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Blanks++
	if opts.OnBlank != nil {
		opts.OnBlank(line)
	}

	if opts.Debug {
		opts.diagnosticf("[BLNK, cd:%d, cm:%d, bk:%d, iscm:%v] %s\n",
			clocFile.Code, clocFile.Comments, clocFile.Blanks, isInComments, lineOrg)
	}
}

func onComment(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Comments++
	if opts.OnComment != nil {
		opts.OnComment(line)
	}

	if opts.Debug {
		opts.diagnosticf("[COMM, cd:%d, cm:%d, bk:%d, iscm:%v] %s\n",
			clocFile.Code, clocFile.Comments, clocFile.Blanks, isInComments, lineOrg)
	}
}

func onCode(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg string) {
	clocFile.Code++
	if opts.OnCode != nil {
		opts.OnCode(line)
	}

	if opts.Debug {
		opts.diagnosticf("[CODE, cd:%d, cm:%d, bk:%d, iscm:%v] %s\n",
			clocFile.Code, clocFile.Comments, clocFile.Blanks, isInComments, lineOrg)
	}
}
