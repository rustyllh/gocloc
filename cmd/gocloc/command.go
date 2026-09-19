package main

import (
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rustyllh/gocloc"
	"github.com/spf13/cobra"
)

// CmdOptions is gocloc command options.
type CmdOptions struct {
	ByFile         bool
	SortTag        string
	OutputType     string
	ExcludeExt     string
	IncludeLang    string
	Match          string
	NotMatch       string
	MatchDir       string
	NotMatchDir    string
	Fullpath       bool
	Debug          bool
	Workers        *int
	SkipDuplicated bool
	ShowLang       bool
	ShowVersion    bool
}

func configureWorkerOptions(opts CmdOptions, clocOpts *gocloc.ClocOptions) error {
	if opts.Workers == nil {
		clocOpts.Workers = automaticWorkerCount()
		return nil
	}
	if *opts.Workers < 0 {
		return fmt.Errorf("--workers must be greater than or equal to zero")
	}
	if *opts.Workers == 0 {
		return fmt.Errorf("--workers must be between 1 and %d", gocloc.MaxWorkers)
	}
	if *opts.Workers > gocloc.MaxWorkers {
		return fmt.Errorf("--workers must not exceed %d", gocloc.MaxWorkers)
	}
	clocOpts.Workers = *opts.Workers
	return nil
}

func automaticWorkerCount() int {
	return runtime.GOMAXPROCS(0)
}

func newRootCommand() *cobra.Command {
	var opts CmdOptions
	var workerCount int
	command := &cobra.Command{
		Use:                "gocloc [OPTIONS] PATH[...]",
		Short:              "A fast, parallel source code line counter",
		Args:               cobra.ArbitraryArgs,
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableSuggestions: true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, paths []string) error {
			if cmd.Flags().Changed("workers") {
				opts.Workers = &workerCount
			}
			clocOpts := gocloc.NewClocOptions()
			if err := configureWorkerOptions(opts, clocOpts); err != nil {
				return err
			}
			if opts.ShowVersion {
				info, _ := debug.ReadBuildInfo()
				_, err := fmt.Fprintln(cmd.OutOrStdout(), versionString(Version, GitCommit, info))
				return err
			}
			if opts.ShowLang {
				languages := gocloc.NewDefinedLanguages()
				_, err := fmt.Fprintln(cmd.OutOrStdout(), languages.GetFormattedString())
				return err
			}
			if len(paths) == 0 {
				return cmd.Help()
			}
			clocOpts.Diagnostics = cmd.ErrOrStderr()
			return runAnalysis(paths, opts, clocOpts, cmd.OutOrStdout())
		},
	}
	command.DisableFlagsInUseLine = true
	flags := command.Flags()
	flags.SetInterspersed(true)
	flags.BoolVarP(
		&opts.ByFile,
		"by-file",
		"f",
		false,
		"report results for every encountered source file",
	)
	opts.SortTag = "code"
	flags.VarP(
		(*sortTagValue)(&opts.SortTag),
		"sort",
		"s",
		"sort based on a certain column [name,files,blank,comment,code]",
	)
	flags.StringVarP(
		&opts.OutputType,
		"output-type",
		"o",
		OutputTypeDefault,
		"output type [values: default,markdown,cloc-xml,sloccount,json]",
	)
	flags.StringVarP(
		&opts.ExcludeExt,
		"exclude-ext",
		"e",
		"",
		"exclude file name extensions (separated commas)",
	)
	flags.StringVarP(
		&opts.IncludeLang,
		"include-lang",
		"l",
		"",
		"include language names (separated commas)",
	)
	flags.StringVar(
		&opts.Match,
		"match",
		"",
		"include file name (regex)",
	)
	flags.StringVar(
		&opts.NotMatch,
		"not-match",
		"",
		"exclude file name (regex)",
	)
	flags.StringVar(
		&opts.MatchDir,
		"match-d",
		"",
		"include dir name (regex)",
	)
	flags.StringVar(
		&opts.NotMatchDir,
		"not-match-d",
		"",
		"exclude dir name (regex)",
	)
	flags.BoolVar(
		&opts.Fullpath,
		"fullpath",
		false,
		"apply match/not-match options to full file paths instead of base names",
	)
	flags.BoolVar(
		&opts.Debug,
		"debug",
		false,
		"dump debug log for developer",
	)
	flags.VarP(
		(*workerCountValue)(&workerCount),
		"workers",
		"w",
		"number of file analysis workers (1-64; default: automatic)",
	)
	flags.BoolVar(
		&opts.SkipDuplicated,
		"skip-duplicated",
		false,
		"skip duplicate-file detection",
	)
	flags.BoolVarP(
		&opts.ShowLang,
		"show-lang",
		"L",
		false,
		"print about all languages and extensions",
	)
	flags.BoolVarP(
		&opts.ShowVersion,
		"version",
		"V",
		false,
		"print version info",
	)
	return command
}

// go-flags parsed integers in base ten; pflag's IntVar instead uses base zero.
// Keep values such as --workers=08 decimal and reject hexadecimal notation.
type workerCountValue int

func (v *workerCountValue) String() string { return strconv.Itoa(int(*v)) }
func (v *workerCountValue) Type() string   { return "int" }

func (v *workerCountValue) Set(value string) error {
	count, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid worker count: %w", err)
	}
	*v = workerCountValue(count)
	return nil
}

// Validate each occurrence during parsing, including before informational exits.
type sortTagValue string

func (v *sortTagValue) String() string { return string(*v) }
func (v *sortTagValue) Type() string   { return "string" }

func (v *sortTagValue) Set(value string) error {
	switch value {
	case "name", "files", "blank", "comment", "code":
		*v = sortTagValue(value)
		return nil
	default:
		return fmt.Errorf("must be one of name, files, blank, comment or code")
	}
}

func runAnalysis(paths []string, opts CmdOptions, clocOpts *gocloc.ClocOptions, out io.Writer) error {
	// check sort tag option with other options
	if opts.ByFile && opts.SortTag == "files" {
		return fmt.Errorf("`--sort files` option cannot be used in conjunction with the `--by-file` option")
	}
	languages := gocloc.NewDefinedLanguages()

	// setup option for exclude extensions
	for _, ext := range strings.Split(opts.ExcludeExt, ",") {
		e, ok := gocloc.Exts[ext]
		if ok {
			clocOpts.ExcludeExts[e] = struct{}{}
		} else {
			clocOpts.ExcludeExts[ext] = struct{}{}
		}
	}

	// directory and file matching options
	for _, filter := range []struct {
		name    string
		pattern string
		target  **regexp.Regexp
	}{
		{name: "match", pattern: opts.Match, target: &clocOpts.ReMatch},
		{name: "not-match", pattern: opts.NotMatch, target: &clocOpts.ReNotMatch},
		{name: "match-d", pattern: opts.MatchDir, target: &clocOpts.ReMatchDir},
		{name: "not-match-d", pattern: opts.NotMatchDir, target: &clocOpts.ReNotMatchDir},
	} {
		if filter.pattern == "" {
			continue
		}
		compiled, err := regexp.Compile(filter.pattern)
		if err != nil {
			return fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
		*filter.target = compiled
	}

	// setup option for include languages
	for _, lang := range strings.Split(opts.IncludeLang, ",") {
		if _, ok := languages.Langs[lang]; ok {
			clocOpts.IncludeLangs[lang] = struct{}{}
		}
	}

	clocOpts.Debug = opts.Debug
	clocOpts.SkipDuplicated = opts.SkipDuplicated
	clocOpts.Fullpath = opts.Fullpath

	processor := gocloc.NewProcessor(languages, clocOpts)
	result, err := processor.Analyze(paths)
	if err != nil {
		return fmt.Errorf("fail gocloc analyze: %w", err)
	}

	return newOutputBuilder(result, &opts, out).WriteResult()
}
