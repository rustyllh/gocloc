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

// NewProcessor returns Processor.
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
	var (
		languages map[string]*Language
		clocFiles map[string]*ClocFile
		err       error
	)
	if resolveWorkerCount(opts) > 1 {
		languages, clocFiles, err = scanAndAnalyzeFiles(paths, p.langs, opts)
	} else {
		languages, err = getAllFiles(paths, p.langs, opts)
		if err == nil {
			clocFiles = analyzeFiles(languages, opts)
		}
	}
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
