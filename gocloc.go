package gocloc

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

// Analyze counts source lines in files and directories using built-in languages.
// Nil options and &Options{} both select automatic concurrency without deduplication.
// Empty paths return an empty result; they do not implicitly scan the current directory.
// Invalid options return an error before scanning. Unreadable files are skipped
// with a diagnostic, following Processor.Analyze's behavior.
// Analyze returns only after all workers and callbacks have completed.
func Analyze(paths []string, opts *Options) (*Result, error) {
	languages := NewDefinedLanguages()
	prepared, err := opts.prepare(languages)
	if err != nil {
		return nil, err
	}
	return NewProcessor(languages, prepared).Analyze(paths)
}

func resolveWorkerCount(opts *ClocOptions) int {
	if opts == nil || opts.Workers <= 1 {
		return 1
	}
	if opts.Workers > MaxWorkers {
		return MaxWorkers
	}
	return opts.Workers
}

// NewProcessor creates a processor with caller-supplied language definitions.
// It retains ClocOptions semantics, including one worker for Workers <= 1.
// For built-in languages and automatic concurrency, use Analyze with Options.
func NewProcessor(langs *DefinedLanguages, options *ClocOptions) *Processor {
	return &Processor{
		langs: langs,
		opts:  options,
	}
}

// Analyze executes gocloc parsing for the directory of the paths argument and returns the result.
func (p *Processor) Analyze(paths []string) (*Result, error) {
	opts := p.opts.withDiagnostics()
	total := NewLanguage("TOTAL", []string{}, [][]string{{"", ""}})
	languages, clocFiles, err := scanAndAnalyzeFiles(paths, p.langs, opts)
	if err != nil {
		return nil, err
	}
	if err := opts.diagnosticError(); err != nil {
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
