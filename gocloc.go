package gocloc

import (
	"sync"
)

// Processor is gocloc analyzing processor.
type Processor struct {
	langs *DefinedLanguages
	opts  *ClocOptions
}

// Result defined processing result.
type Result struct {
	Total         *Language
	Files         map[string]*ClocFile
	Languages     map[string]*Language
	MaxPathLength int
}

type analysisJob struct {
	file        string
	languageKey string
	language    *Language
}

type analysisResult struct {
	file        string
	languageKey string
	clocFile    *ClocFile
}

type languageCounts struct {
	code     int32
	comments int32
	blanks   int32
}

func resolveWorkerCount(opts *ClocOptions) int {
	if opts == nil || opts.Debug || opts.OnCode != nil || opts.OnBlank != nil || opts.OnComment != nil {
		return 1
	}
	if opts.Workers > 1 {
		if opts.Workers > MaxWorkers {
			return MaxWorkers
		}
		return opts.Workers
	}
	return 1
}

func analyzeFiles(languages map[string]*Language, opts *ClocOptions) map[string]*ClocFile {
	fileCount := 0
	for _, language := range languages {
		fileCount += len(language.Files)
	}
	clocFiles := make(map[string]*ClocFile, fileCount)
	counts := make(map[string]languageCounts, len(languages))

	addResult := func(result analysisResult) {
		clocFiles[result.file] = result.clocFile
		count := counts[result.languageKey]
		count.code += result.clocFile.Code
		count.comments += result.clocFile.Comments
		count.blanks += result.clocFile.Blanks
		counts[result.languageKey] = count
	}

	workers := resolveWorkerCount(opts)
	if workers == 1 {
		for languageKey, language := range languages {
			for _, file := range language.Files {
				clocFile := AnalyzeFile(file, language, opts)
				clocFile.Lang = language.Name
				addResult(analysisResult{file: file, languageKey: languageKey, clocFile: clocFile})
			}
		}
	} else {
		jobs := make(chan analysisJob, 2*workers)
		results := make(chan analysisResult, 2*workers)
		var workersGroup sync.WaitGroup
		for range workers {
			workersGroup.Add(1)
			go func() {
				defer workersGroup.Done()
				for job := range jobs {
					clocFile := AnalyzeFile(job.file, job.language, opts)
					clocFile.Lang = job.language.Name
					results <- analysisResult{file: job.file, languageKey: job.languageKey, clocFile: clocFile}
				}
			}()
		}

		go func() {
			for languageKey, language := range languages {
				for _, file := range language.Files {
					jobs <- analysisJob{file: file, languageKey: languageKey, language: language}
				}
			}
			close(jobs)
			workersGroup.Wait()
			close(results)
		}()

		for result := range results {
			addResult(result)
		}
	}

	for languageKey, count := range counts {
		language := languages[languageKey]
		language.Code += count.code
		language.Comments += count.comments
		language.Blanks += count.blanks
	}

	return clocFiles
}

// NewProcessor returns Processor.
func NewProcessor(langs *DefinedLanguages, options *ClocOptions) *Processor {
	return &Processor{
		langs: langs,
		opts:  options,
	}
}

// Analyze executes gocloc parsing for the directory of the paths argument and returns the result.
func (p *Processor) Analyze(paths []string) (*Result, error) {
	total := NewLanguage("TOTAL", []string{}, [][]string{{"", ""}})
	languages, err := getAllFiles(paths, p.langs, p.opts)
	if err != nil {
		return nil, err
	}
	maxPathLen := 0
	for _, lang := range languages {
		for _, file := range lang.Files {
			l := len(file)
			if maxPathLen < l {
				maxPathLen = l
			}
		}
	}
	clocFiles := analyzeFiles(languages, p.opts)

	for _, language := range languages {
		files := int32(len(language.Files))
		if len(language.Files) <= 0 {
			continue
		}

		total.Total += files
		total.Blanks += language.Blanks
		total.Comments += language.Comments
		total.Code += language.Code
	}

	return &Result{
		Total:         total,
		Files:         clocFiles,
		Languages:     languages,
		MaxPathLength: maxPathLen,
	}, nil
}
