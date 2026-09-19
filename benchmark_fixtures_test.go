package gocloc

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const benchmarkFixtureVersion = "v1"

type benchmarkFixtureSpec struct {
	name     string
	files    int
	unique   int
	code     int32
	comments int32
	blanks   int32
}

func benchmarkFixtureSpecs() []benchmarkFixtureSpec {
	return []benchmarkFixtureSpec{
		{name: "many_small", files: 512, unique: 512, code: 17, comments: 1, blanks: 1},
		{name: "few_large", files: 4, unique: 4, code: 49153, comments: 1, blanks: 1},
		{name: "duplicates", files: 512, unique: 32, code: 17, comments: 1, blanks: 1},
		{name: "comments", files: 32, unique: 32, code: 1, comments: 12289, blanks: 4097},
		{name: "long_lines", files: 8, unique: 8, code: 5, comments: 1, blanks: 1},
	}
}

type benchmarkFixture struct {
	root   string
	bytes  int64
	digest string
	spec   benchmarkFixtureSpec
}

func writeBenchmarkFixture(tb testing.TB, spec benchmarkFixtureSpec) benchmarkFixture {
	tb.Helper()
	fixture := benchmarkFixture{root: tb.TempDir(), spec: spec}
	var body string
	switch spec.name {
	case "many_small", "duplicates":
		body = strings.Repeat("var value = 123456789\n", 16)
	case "few_large":
		body = strings.Repeat("var value = 123456789\n", 49152)
	case "comments":
		body = strings.Repeat("/* block\ncomment text\n*/\n\n", 4096)
	case "long_lines":
		body = strings.Repeat("var value = \""+strings.Repeat("x", 128*1024)+"\"\n", 4)
	default:
		tb.Fatalf("unknown fixture %q", spec.name)
	}
	digest := sha256.New()
	for i := range spec.files {
		// Forward slashes and fixed-width names make the fingerprint and first
		// duplicate independent of the temporary directory and operating system.
		name := fmt.Sprintf("dir%03d/file%06d.go", i/16, i)
		content := []byte(fmt.Sprintf("package fixture\n// fixture %08d\n\n", i%spec.unique) + body)
		path := filepath.Join(fixture.root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			tb.Fatal(err)
		}
		if _, err := fmt.Fprintf(digest, "%s\x00%d\x00", name, len(content)); err != nil {
			tb.Fatal(err)
		}
		if _, err := digest.Write(content); err != nil {
			tb.Fatal(err)
		}
		fixture.bytes += int64(len(content))
	}
	fixture.digest = fmt.Sprintf("%x", digest.Sum(nil))
	return fixture
}

func (fixture benchmarkFixture) verify(tb testing.TB, result *Result, skipDuplicated bool) {
	tb.Helper()
	files := fixture.spec.unique
	if skipDuplicated {
		files = fixture.spec.files
	}
	wantCode := int32(files) * fixture.spec.code
	wantComments := int32(files) * fixture.spec.comments
	wantBlanks := int32(files) * fixture.spec.blanks
	if result == nil || result.Total == nil {
		tb.Fatal("missing analysis result")
	}
	if len(result.Files) != files || result.Total.Total != int32(files) {
		tb.Fatalf("files = %d (total %d), want %d", len(result.Files), result.Total.Total, files)
	}
	if result.Total.Code != wantCode || result.Total.Comments != wantComments {
		tb.Fatalf("code/comments = %d/%d, want %d/%d",
			result.Total.Code, result.Total.Comments, wantCode, wantComments)
	}
	if result.Total.Blanks != wantBlanks {
		tb.Fatalf("blanks = %d, want %d", result.Total.Blanks, wantBlanks)
	}
	for i := range files {
		name := fmt.Sprintf("dir%03d/file%06d.go", i/16, i)
		file := result.Files[filepath.Join(fixture.root, filepath.FromSlash(name))]
		if file == nil || file.Lang != "Go" {
			tb.Fatalf("missing Go file %s (first-discovered copies must be retained)", name)
		}
	}
}

func TestBenchmarkFixtures(t *testing.T) {
	for _, spec := range benchmarkFixtureSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			fixture := writeBenchmarkFixture(t, spec)
			copy := writeBenchmarkFixture(t, spec)
			if fixture.digest != copy.digest || fixture.bytes != copy.bytes {
				t.Fatal("fixture contents depend on the temporary directory")
			}
			t.Logf("fixture=%s/%s files=%d bytes=%d sha256=%s",
				benchmarkFixtureVersion, spec.name, spec.files, fixture.bytes, fixture.digest)
			for _, workers := range []int{1, 8} {
				for _, dedup := range []bool{true, false} {
					t.Run(fmt.Sprintf("workers=%d/dedup=%t", workers, dedup), func(t *testing.T) {
						processor := NewProcessor(NewDefinedLanguages(), newBenchmarkOptions(workers, !dedup))
						result, err := processor.Analyze([]string{fixture.root})
						if err != nil {
							t.Fatal(err)
						}
						fixture.verify(t, result, !dedup)
					})
				}
			}
		})
	}
}

// BenchmarkProcessorFixtures needs no external checkout or network. Each leaf
// measures a complete Processor.Analyze call on a warmed, deterministic corpus.
func BenchmarkProcessorFixtures(b *testing.B) {
	for _, spec := range benchmarkFixtureSpecs() {
		b.Run(benchmarkFixtureVersion+"/"+spec.name, func(b *testing.B) {
			fixture := writeBenchmarkFixture(b, spec)
			b.Logf("files=%d bytes=%d sha256=%s GOMAXPROCS=%d",
				spec.files, fixture.bytes, fixture.digest, runtime.GOMAXPROCS(0))
			for _, workers := range []int{1, 2, 4, 8} {
				for _, dedup := range []bool{true, false} {
					b.Run(fmt.Sprintf("workers=%d/dedup=%t", workers, dedup), func(b *testing.B) {
						benchmarkFixtureAnalyze(b, fixture, workers, !dedup)
					})
				}
			}
		})
	}
}

func benchmarkFixtureAnalyze(b *testing.B, fixture benchmarkFixture, workers int, skipDuplicated bool) {
	b.StopTimer()
	paths := []string{fixture.root}
	processor := NewProcessor(NewDefinedLanguages(), newBenchmarkOptions(workers, skipDuplicated))
	result, err := processor.Analyze(paths)
	if err != nil {
		b.Fatal(err)
	}
	fixture.verify(b, result, skipDuplicated)
	b.ReportAllocs()
	b.SetBytes(fixture.bytes)
	b.ResetTimer()
	b.StartTimer()
	for range b.N {
		processor := NewProcessor(NewDefinedLanguages(), newBenchmarkOptions(workers, skipDuplicated))
		result, err = processor.Analyze(paths)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	fixture.verify(b, result, skipDuplicated)
}
