package gocloc

import (
	"crypto/md5"
	"errors"
	"fmt"
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
	events      *callbackEvents
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

// All analysis uses this pipeline, including callbacks, debug and one worker.
// With deduplication, callbacks are recorded during analysis and dispatched only
// for retained files. Debug records still describe work done on excluded copies.
func scanAndAnalyzeFiles(
	paths []string,
	languages *DefinedLanguages,
	opts *ClocOptions,
) (map[string]*Language, map[string]*ClocFile, error) {
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

	var callbacks *callbackDispatcher
	hasCallbacks := opts.OnCode != nil || opts.OnBlank != nil || opts.OnComment != nil
	if hasCallbacks && !opts.SkipDuplicated {
		callbacks = newCallbackDispatcher(workers, opts, tokens)
	}

	var workerGroup sync.WaitGroup
	for range workers {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			reader := newLineReader(nil)
			for candidate := range candidates {
				analysisOpts := opts
				var events *callbackEvents
				if callbacks != nil {
					events = &callbackEvents{memoryLimit: callbackMemoryBudget / windowSize}
					analysisOpts = events.capture(opts)
				}
				item := detectAndAnalyzeFile(
					candidate,
					languages,
					analysisOpts,
					reader,
				)
				if events != nil {
					item.events = events
					if err := events.finish(); err != nil {
						item.err = errors.Join(
							item.err,
							fmt.Errorf("buffer callbacks for %q: %w", candidate.path, err),
						)
					}
				}
				results <- item
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
				if current.ignored && opts.Debug {
					opts.diagnosticf("[SKIP] file=%q reason=\"duplicate content\"\n", current.path)
				}
			}
			if !current.ignored {
				addFileToResult(result, languages, current.languageKey, current.path)
				language := result[current.languageKey]
				addFileCounts(language, current.clocFile)
				clocFiles[current.path] = current.clocFile
			}
			if current.events != nil && !current.ignored {
				// The callback worker owns the events and the token until replay
				// and cleanup finish, so slow callbacks also bound memory use.
				callbacks.jobs <- callbackJob{path: current.path, events: current.events}
			} else {
				if current.events != nil {
					if err := current.events.discard(); err != nil {
						callbacks.report(fmt.Errorf("discard callbacks for %q: %w", current.path, err))
					}
				}
				tokens <- struct{}{}
			}
			nextSequence++
		}
	}

	var callbackErr error
	if callbacks != nil {
		callbackErr = callbacks.finish()
	}
	return result, clocFiles, errors.Join(<-walkErrors, callbackErr)
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
