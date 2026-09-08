package gocloc

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

func InsertPipesInTheMiddle(input string) string {
	var output strings.Builder
	re := regexp.MustCompile(`\s+`)
	words := strings.Fields(input)
	spaces := re.FindAllString(input, -1)

	for i := 0; i < len(words); i++ {
		if i < len(spaces) {
			middle := (len(spaces[i]) / 2) - 1
			output.WriteString(words[i] + strings.Repeat(" ", middle) + "|" + strings.Repeat(" ", middle))
		} else {
			output.WriteString(words[i] + " |")
		}
	}

	return output.String()
}

func trimBOM(line string) string {
	l := len(line)
	if l >= 3 {
		if line[0] == 0xef && line[1] == 0xbb && line[2] == 0xbf {
			trimLine := line[3:]
			return trimLine
		}
	}
	return line
}

func containsComment(line string, multiLines [][]string) bool {
	for _, comments := range multiLines {
		for _, comm := range comments {
			if strings.Contains(line, comm) {
				return true
			}
		}
	}
	return false
}

func nextRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func checkMD5Sum(path string, fileCache map[string]struct{}) (ignore bool) {
	digest, ignored := hashFile(path)
	if ignored {
		return true
	}
	c := string(digest[:])
	if _, ok := fileCache[c]; ok {
		return true
	}

	fileCache[c] = struct{}{}
	return false
}

func hashFile(path string) (digest [md5.Size]byte, ignored bool) {
	file, err := os.Open(path)
	if err != nil {
		return digest, true
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return digest, true
	}
	copy(digest[:], hash.Sum(nil))
	return digest, false
}

func isVCSDir(path string) bool {
	if len(path) > 1 && path[0] == os.PathSeparator {
		path = path[1:]
	}
	vcsDirs := []string{".bzr", ".cvs", ".hg", ".git", ".svn"}
	for _, dir := range vcsDirs {
		if strings.Contains(path, dir) {
			return true
		}
	}
	return false
}

func checkDefaultIgnore(path string, info os.FileInfo, isVCS bool) bool {
	if info.IsDir() {
		// directory is ignored
		return true
	}
	if !isVCS && isVCSDir(path) {
		// vcs file or directory is ignored
		return true
	}

	return false
}

func checkOptionMatch(path string, info os.FileInfo, opts *ClocOptions) bool {
	// check match directory & file options
	targetFile := info.Name()
	if opts.Fullpath {
		targetFile = path
	}

	if opts.ReNotMatch != nil && opts.ReNotMatch.MatchString(targetFile) {
		return false
	}
	if opts.ReMatch != nil && !opts.ReMatch.MatchString(targetFile) {
		return false
	}

	dir := filepath.Dir(path)
	if opts.ReNotMatchDir != nil && opts.ReNotMatchDir.MatchString(dir) {
		return false
	}

	if opts.ReMatchDir != nil && !opts.ReMatchDir.MatchString(dir) {
		return false
	}

	return true
}

type md5Candidate struct {
	sequence    uint64
	path        string
	languageKey string
}

type md5Result struct {
	md5Candidate
	digest   [md5.Size]byte
	clocFile *ClocFile
	ignored  bool
}

func parallelResultWindowSize(workers int) int {
	windowSize := 32 * workers
	if windowSize < 256 {
		return 256
	}
	return windowSize
}

func addFileToResult(result map[string]*Language, languages *DefinedLanguages, languageKey, path string) {
	if _, ok := result[languageKey]; !ok {
		definedLang := NewLanguage(
			languages.Langs[languageKey].Name,
			languages.Langs[languageKey].lineComments,
			languages.Langs[languageKey].multiLines,
		)
		if len(languages.Langs[languageKey].regexLineComments) > 0 {
			definedLang.regexLineComments = languages.Langs[languageKey].regexLineComments
		}
		result[languageKey] = definedLang
	}
	result[languageKey].Files = append(result[languageKey].Files, path)
}

func getAllFilesParallelMD5(paths []string, languages *DefinedLanguages, opts *ClocOptions) (map[string]*Language, map[string]*ClocFile, error) {
	workers := resolveWorkerCount(opts)
	result := make(map[string]*Language)
	clocFiles := make(map[string]*ClocFile)
	fileCache := make(map[string]struct{})
	windowSize := parallelResultWindowSize(workers)
	tokens := make(chan struct{}, windowSize)
	for range windowSize {
		tokens <- struct{}{}
	}
	candidates := make(chan md5Candidate, windowSize)
	results := make(chan md5Result, windowSize)

	var workerGroup sync.WaitGroup
	for range workers {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for candidate := range candidates {
				language := languages.Langs[candidate.languageKey]
				clocFile, digest, ignored := analyzeFileAndHash(candidate.path, language, opts)
				clocFile.Lang = language.Name
				results <- md5Result{md5Candidate: candidate, digest: digest, clocFile: clocFile, ignored: ignored}
			}
		}()
	}

	walkErrors := make(chan error, 1)
	go func() {
		var walkErr error
		sequence := uint64(0)
		for _, root := range paths {
			vcsInRoot := isVCSDir(root)
			walkErr = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
					return nil
				}
				if checkDefaultIgnore(path, info, vcsInRoot) || !checkOptionMatch(path, info, opts) {
					return nil
				}
				ext, ok := getFileType(path, opts)
				if !ok {
					return nil
				}
				languageKey, ok := Exts[ext]
				if !ok {
					return nil
				}
				if _, ok := opts.ExcludeExts[languageKey]; ok {
					return nil
				}
				if len(opts.IncludeLangs) != 0 {
					if _, ok := opts.IncludeLangs[languageKey]; !ok {
						return nil
					}
				}
				<-tokens
				candidates <- md5Candidate{sequence: sequence, path: path, languageKey: languageKey}
				sequence++
				return nil
			})
			if walkErr != nil {
				break
			}
		}
		close(candidates)
		walkErrors <- walkErr
	}()

	go func() {
		workerGroup.Wait()
		close(results)
	}()

	pending := make(map[uint64]md5Result)
	nextSequence := uint64(0)
	for resultItem := range results {
		pending[resultItem.sequence] = resultItem
		for {
			current, ok := pending[nextSequence]
			if !ok {
				break
			}
			delete(pending, nextSequence)
			if !current.ignored {
				cacheKey := string(current.digest[:])
				if _, duplicated := fileCache[cacheKey]; duplicated {
					if opts.Debug {
						fmt.Printf("[ignore=%v] find same md5\n", current.path)
					}
				} else {
					fileCache[cacheKey] = struct{}{}
					addFileToResult(result, languages, current.languageKey, current.path)
					language := result[current.languageKey]
					language.Code += current.clocFile.Code
					language.Comments += current.clocFile.Comments
					language.Blanks += current.clocFile.Blanks
					clocFiles[current.path] = current.clocFile
				}
			}
			tokens <- struct{}{}
			nextSequence++
		}
	}

	return result, clocFiles, <-walkErrors
}

// getAllFiles return all the files to be analyzed in paths.
func getAllFiles(paths []string, languages *DefinedLanguages, opts *ClocOptions) (result map[string]*Language, err error) {
	result = make(map[string]*Language, 0)
	fileCache := make(map[string]struct{})

	for _, root := range paths {
		vcsInRoot := isVCSDir(root)
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				return nil
			}
			if ignore := checkDefaultIgnore(path, info, vcsInRoot); ignore {
				return nil
			}

			// check match & not-match directory
			if match := checkOptionMatch(path, info, opts); !match {
				return nil
			}

			if ext, ok := getFileType(path, opts); ok {
				if targetExt, ok := Exts[ext]; ok {
					// check exclude extension
					if _, ok := opts.ExcludeExts[targetExt]; ok {
						return nil
					}

					if len(opts.IncludeLangs) != 0 {
						if _, ok = opts.IncludeLangs[targetExt]; !ok {
							return nil
						}
					}

					if !opts.SkipDuplicated {
						ignore := checkMD5Sum(path, fileCache)
						if ignore {
							if opts.Debug {
								fmt.Printf("[ignore=%v] find same md5\n", path)
							}
							return nil
						}
					}

					addFileToResult(result, languages, targetExt, path)
				}
			}
			return nil
		})
	}
	return
}
