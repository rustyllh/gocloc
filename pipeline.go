package gocloc

import (
	"crypto/md5"
	"sync"
)

type fileCandidate struct {
	sequence uint64
	path     string
}

type fileAnalysisResult struct {
	fileCandidate
	languageKey string
	err         error
	digest      [md5.Size]byte
	clocFile    *ClocFile
	ignored     bool
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

func scanAndAnalyzeFiles(paths []string, languages *DefinedLanguages, opts *ClocOptions) (map[string]*Language, map[string]*ClocFile, error) {
	workers := resolveWorkerCount(opts)
	result := make(map[string]*Language)
	clocFiles := make(map[string]*ClocFile)
	fileCache := make(map[string]struct{})
	windowSize := parallelResultWindowSize(workers)
	tokens := make(chan struct{}, windowSize)
	for range windowSize {
		tokens <- struct{}{}
	}
	candidates := make(chan fileCandidate, windowSize)
	results := make(chan fileAnalysisResult, windowSize)

	var workerGroup sync.WaitGroup
	for range workers {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for candidate := range candidates {
				results <- detectAndAnalyzeFile(candidate, languages, opts)
			}
		}()
	}

	walkErrors := make(chan error, 1)
	go func() {
		var walkErr error
		sequence := uint64(0)
		for _, root := range paths {
			walkErr = walkCandidateFiles(root, opts, func(path string) error {
				<-tokens
				candidates <- fileCandidate{sequence: sequence, path: path}
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

	pending := make(map[uint64]fileAnalysisResult)
	nextSequence := uint64(0)
	for resultItem := range results {
		pending[resultItem.sequence] = resultItem
		for {
			current, ok := pending[nextSequence]
			if !ok {
				break
			}
			delete(pending, nextSequence)
			if current.err != nil {
				opts.warn(current.err)
				current.ignored = true
			}
			if !current.ignored && !opts.SkipDuplicated {
				current.ignored = duplicateDigest(current.digest, fileCache)
			}
			if !current.ignored {
				addFileToResult(result, languages, current.languageKey, current.path)
				language := result[current.languageKey]
				addFileCounts(language, current.clocFile)
				clocFiles[current.path] = current.clocFile
			}
			tokens <- struct{}{}
			nextSequence++
		}
	}

	return result, clocFiles, <-walkErrors
}

// getAllFiles discovers and deduplicates before analysis so excluded copies
// never trigger callbacks. Debug and callbacks intentionally keep this ordering.
func getAllFiles(paths []string, languages *DefinedLanguages, opts *ClocOptions) (map[string]*Language, error) {
	result := make(map[string]*Language)
	fileCache := make(map[string]struct{})
	for _, root := range paths {
		err := walkCandidateFiles(root, opts, func(path string) error {
			ext, recognized := getFileType(path, opts)
			languageKey, known := Exts[ext]
			if !recognized || !known {
				return nil
			}
			if !includeLanguage(languageKey, opts) {
				return nil
			}
			if !opts.SkipDuplicated {
				digest, err := hashFile(path)
				if err != nil {
					opts.warn(err)
					return nil
				}
				if duplicateDigest(digest, fileCache) {
					if opts.Debug {
						opts.diagnosticf("[ignore=%v] find same md5\n", path)
					}
					return nil
				}
			}
			addFileToResult(result, languages, languageKey, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func includeLanguage(languageKey string, opts *ClocOptions) bool {
	if _, excluded := opts.ExcludeExts[languageKey]; excluded {
		return false
	}
	if len(opts.IncludeLangs) == 0 {
		return true
	}
	_, included := opts.IncludeLangs[languageKey]
	return included
}

func addFileCounts(language *Language, file *ClocFile) {
	language.Code += file.Code
	language.Comments += file.Comments
	language.Blanks += file.Blanks
}

// The parallel path uses scanAndAnalyzeFiles. This path remains synchronous to
// preserve callback order and avoid analyzing duplicates before invoking them.
func analyzeFiles(languages map[string]*Language, opts *ClocOptions) map[string]*ClocFile {
	fileCount := 0
	for _, language := range languages {
		fileCount += len(language.Files)
	}
	clocFiles := make(map[string]*ClocFile, fileCount)
	for _, language := range languages {
		kept := language.Files[:0]
		for _, path := range language.Files {
			file, err := analyzeFile(path, language, opts)
			if err != nil {
				opts.warn(err)
				continue
			}
			kept = append(kept, path)
			clocFiles[path] = file
			addFileCounts(language, file)
		}
		language.Files = kept
	}
	return clocFiles
}
