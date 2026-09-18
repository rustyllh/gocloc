package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rustyllh/gocloc"
	"github.com/spf13/cobra"
)

// Version is version string for gocloc command
var Version string

// GitCommit is git commit hash string for gocloc command
var GitCommit string

func versionString(version, commit string, info *debug.BuildInfo) string {
	if info != nil {
		if version == "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		if commit == "" {
			modified := false
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					commit = setting.Value
				case "vcs.modified":
					modified = setting.Value == "true"
				}
			}
			if modified && commit != "" {
				commit += "-dirty"
			}
		}
	}
	if version == "" {
		version = "devel"
	}
	if commit == "" {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}

// OutputTypeDefault is cloc's text output format for --output-type option
const OutputTypeDefault string = "default"

// OutputTypeClocXML is Cloc's XML output format for --output-type option
const OutputTypeClocXML string = "cloc-xml"

// OutputTypeSloccount is Sloccount output format for --output-type option
const OutputTypeSloccount string = "sloccount"

// OutputTypeJSON is JSON output format for --output-type option
const OutputTypeJSON string = "json"

// OutputTypeMarkdown is Markdown output format for --output-type option
const OutputTypeMarkdown string = "markdown"

const (
	fileHeader             string = "File"
	languageHeader         string = "Language"
	commonHeader           string = "files          blank        comment           code"
	defaultOutputSeparator string = "-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------"
)

var rowLen = 79

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

type outputBuilder struct {
	opts   *CmdOptions
	result *gocloc.Result
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

func newOutputBuilder(result *gocloc.Result, opts *CmdOptions) *outputBuilder {
	return &outputBuilder{
		opts,
		result,
	}
}

func (o *outputBuilder) WriteHeader() {
	maxPathLen := o.result.MaxPathLength
	headerLen := 28
	header := languageHeader

	if o.opts.ByFile {
		headerLen = maxPathLen + 1
		rowLen = maxPathLen + len(commonHeader) + 2
		header = fileHeader
	}

	if o.opts.OutputType == OutputTypeDefault {
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
		fmt.Printf("%-[2]*[1]s %[3]s\n", header, headerLen, commonHeader)
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		allHeaders := fmt.Sprintf("%s%s%s", header, strings.Repeat(" ", headerLen), commonHeader)
		headerString := "| " + gocloc.InsertPipesInTheMiddle(allHeaders)
		fmt.Println(headerString)

		for i := 0; i < len(headerString); i++ {
			if headerString[i] == '|' {
				fmt.Print("|")
			} else {
				if i == 1 {
					// Align the first column to the left
					fmt.Print(":")
				} else {
					// Align the other columns to the right
					if headerString[i+1] == '|' && i > headerLen {
						fmt.Print(":")
					} else {
						fmt.Print("-")
					}
				}
			}
		}

		fmt.Println()
	}
}

func (o *outputBuilder) WriteFooter() {
	total := o.result.Total
	maxPathLen := o.result.MaxPathLength

	if o.opts.OutputType == OutputTypeDefault {
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
		if o.opts.ByFile {
			fmt.Printf("%-[1]*[2]v %6[3]v %14[4]v %14[5]v %14[6]v\n",
				maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			fmt.Printf("%-27v %6v %14v %14v %14v\n",
				"TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		if o.opts.ByFile {
			fmt.Printf("| %-[1]*[2]v |%10v|%12v|%14v|%8v |\n", maxPathLen, "", "", "", "", "")
			fmt.Printf("| %-[1]*[2]v |%9v |%11v |%13v |%8v |\n", maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			fmt.Printf("| %21v|%22v|%12v|%14v|%8v |\n", "", "", "", "", "")
			fmt.Printf("| %20v |%21v |%11v |%13v |%8v |\n", "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
	}
}

func writeResultWithByFile(opts *CmdOptions, result *gocloc.Result) {
	clocFiles := result.Files
	total := result.Total
	maxPathLen := result.MaxPathLength

	var sortedFiles gocloc.ClocFiles
	for _, file := range clocFiles {
		sortedFiles = append(sortedFiles, *file)
	}
	switch opts.SortTag {
	case "name":
		sortedFiles.SortByName()
	case "comment":
		sortedFiles.SortByComments()
	case "blank":
		sortedFiles.SortByBlanks()
	default:
		sortedFiles.SortByCode()
	}

	switch opts.OutputType {
	case OutputTypeClocXML:
		t := gocloc.XMLTotalFiles{
			Code:    total.Code,
			Comment: total.Comments,
			Blank:   total.Blanks,
		}
		f := &gocloc.XMLResultFiles{
			Files: sortedFiles,
			Total: t,
		}
		xmlResult := gocloc.XMLResult{
			XMLFiles: f,
		}
		xmlResult.Encode()
	case OutputTypeSloccount:
		for _, file := range sortedFiles {
			p := ""
			if strings.HasPrefix(file.Name, "./") || string(file.Name[0]) == "/" {
				splitPaths := strings.Split(file.Name, string(os.PathSeparator))
				if len(splitPaths) >= 3 {
					p = splitPaths[1]
				}
			}
			fmt.Printf("%v\t%v\t%v\t%v\n",
				file.Code, file.Lang, p, file.Name)
		}
	case OutputTypeJSON:
		jsonResult := gocloc.NewJSONFilesResultFromCloc(total, sortedFiles)
		buf, err := json.Marshal(jsonResult)
		if err != nil {
			fmt.Println(err)
			panic("json marshal error")
		}
		os.Stdout.Write(buf)
	case OutputTypeMarkdown:
		for _, file := range sortedFiles {
			clocFile := file
			fmt.Printf("| %-[1]*[2]s |%8[3]v  |%11[4]v |%13[5]v |%8[6]v |\n",
				maxPathLen, file.Name, 1, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}

	default:
		for _, file := range sortedFiles {
			clocFile := file
			fmt.Printf("%-[1]*[2]s %21[3]v %14[4]v %14[5]v\n",
				maxPathLen, file.Name, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}
	}
}

func (o *outputBuilder) WriteResult() {
	// write header
	o.WriteHeader()

	clocLangs := o.result.Languages
	total := o.result.Total

	if o.opts.ByFile {
		writeResultWithByFile(o.opts, o.result)
	} else {
		var sortedLanguages gocloc.Languages
		for _, language := range clocLangs {
			if len(language.Files) != 0 {
				sortedLanguages = append(sortedLanguages, *language)
			}
		}
		switch o.opts.SortTag {
		case "name":
			sortedLanguages.SortByName()
		case "files":
			sortedLanguages.SortByFiles()
		case "comment":
			sortedLanguages.SortByComments()
		case "blank":
			sortedLanguages.SortByBlanks()
		default:
			sortedLanguages.SortByCode()
		}

		switch o.opts.OutputType {
		case OutputTypeClocXML:
			xmlResult := gocloc.NewXMLResultFromCloc(total, sortedLanguages, gocloc.XMLResultWithLangs)
			xmlResult.Encode()
		case OutputTypeJSON:
			jsonResult := gocloc.NewJSONLanguagesResultFromCloc(total, sortedLanguages)
			buf, err := json.Marshal(jsonResult)
			if err != nil {
				fmt.Println(err)
				panic("json marshal error")
			}
			os.Stdout.Write(buf)
		case OutputTypeMarkdown:
			for _, language := range sortedLanguages {
				fmt.Printf("| %-20v |%21v |%11v |%13v |%8v |\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		default:
			for _, language := range sortedLanguages {
				fmt.Printf("%-27v %6v %14v %14v %14v\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		}
	}

	// write footer
	o.WriteFooter()
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
			return runAnalysis(paths, opts, clocOpts)
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

func runAnalysis(paths []string, opts CmdOptions, clocOpts *gocloc.ClocOptions) error {
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

	builder := newOutputBuilder(result, &opts)
	builder.WriteResult()
	return nil
}
