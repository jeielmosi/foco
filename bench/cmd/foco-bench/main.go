// Command foco-bench is foco's standard benchmark. It builds two foco
// commits, runs the same searches with both against a deterministic fake
// provider on corpora pinned in bench/corpora.json, and writes a Markdown
// report in the format used in pull requests plus the raw results as JSON.
//
//	go run ./bench/cmd/foco-bench fetch -suite small
//	go run ./bench/cmd/foco-bench run -base main -head HEAD -suite small
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[foco-bench] "+format+"\n", args...)
}

func main() {
	if len(os.Args) < 2 {
		usageExit()
	}
	var err error
	switch os.Args[1] {
	case "fetch":
		err = fetchCommand(os.Args[2:])
	case "run":
		err = runCommand(os.Args[2:])
	default:
		usageExit()
	}
	if err != nil {
		logf("%v", err)
		os.Exit(1)
	}
}

func usageExit() {
	fmt.Fprintln(os.Stderr, "usage: foco-bench fetch|run [flags]  (foco-bench run -h for flags)")
	os.Exit(2)
}

func defaultCache() string {
	if dir := os.Getenv("FOCO_BENCH_CACHE"); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "foco-bench")
}

type common struct {
	manifest string
	suite    string
	corpora  string
	cache    string
}

func (c *common) register(flags *flag.FlagSet) {
	flags.StringVar(&c.manifest, "manifest", "bench/corpora.json", "corpus manifest")
	flags.StringVar(&c.suite, "suite", "small", "suite to run: small, medium (includes small) or large (everything)")
	flags.StringVar(&c.corpora, "corpus", "", "comma-separated corpus names; overrides -suite")
	flags.StringVar(&c.cache, "cache", defaultCache(), "cache for corpora, worktrees and binaries (or FOCO_BENCH_CACHE)")
}

func (c *common) selected() ([]corpus, error) {
	loaded, err := loadManifest(c.manifest)
	if err != nil {
		return nil, err
	}
	var names []string
	if c.corpora != "" {
		names = strings.Split(c.corpora, ",")
	}
	return selectCorpora(loaded.Corpora, c.suite, names)
}

func fetchCommand(args []string) error {
	var options common
	flags := flag.NewFlagSet("fetch", flag.ExitOnError)
	options.register(flags)
	flags.Parse(args)
	selected, err := options.selected()
	if err != nil {
		return err
	}
	for _, entry := range selected {
		dir, err := entry.fetch(options.cache, logf)
		if err != nil {
			return err
		}
		logf("%s ready at %s", entry.Name, dir)
	}
	return nil
}

func runCommand(args []string) error {
	var options common
	var provider providerOptions
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	options.register(flags)
	repo := flags.String("repo", ".", "foco repository to build from")
	repoURL := flags.String("repo-url", "", "web URL used for commit links (default: derived from origin)")
	baseRef := flags.String("base", "main", "base ref")
	headRef := flags.String("head", "HEAD", "head ref; \".\" builds the working tree with uncommitted changes")
	runs := flags.Int("runs", 3, "timed runs per build and search")
	tags := flags.String("tags", "", "build tags for both builds, e.g. vectorscan (needs cgo and libhs)")
	outDir := flags.String("out", "bench/results", "directory for report.md and results.json")
	detector := flags.Bool("detect", true, "also benchmark the secret detector alone over the suite")
	flags.DurationVar(&provider.Latency, "latency", 300*time.Millisecond, "fake provider latency per response")
	flags.DurationVar(&provider.LatencyPerKB, "latency-per-kb", 0, "extra fake provider latency per KB received")
	flags.IntVar(&provider.MaxQuestions, "max-questions", 100, "reject requests with more questions (0 disables)")
	flags.IntVar(&provider.MaxContext, "max-context", 0, "reject requests whose state plus longest question exceeds this many bytes (0 disables)")
	flags.IntVar(&provider.WarnContext, "warn-context", 40000, "count requests whose state plus longest question exceeds this many bytes (0 disables)")
	flags.BoolVar(&provider.TLS, "tls", true, "serve the fake provider over TLS")
	flags.Parse(args)
	if *runs < 1 {
		return errors.New("-runs must be at least 1")
	}
	selected, err := options.selected()
	if err != nil {
		return err
	}
	root, err := git(*repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if *repoURL == "" {
		*repoURL = webURL(root)
	}
	for _, dir := range []string{options.cache, filepath.Join(options.cache, "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	scratch, err := os.MkdirTemp("", "foco-bench-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	base, err := prepareBuild("base", *baseRef, root, options.cache, *tags, logf)
	if err != nil {
		return err
	}
	head, err := prepareBuild("head", *headRef, root, options.cache, *tags, logf)
	if err != nil {
		return err
	}
	dirs := map[string]string{}
	for _, entry := range selected {
		if dirs[entry.Name], err = entry.fetch(options.cache, logf); err != nil {
			return err
		}
	}
	fake, err := startProvider(provider, scratch)
	if err != nil {
		return err
	}
	defer fake.close()
	env, err := searchEnv(scratch, fake)
	if err != nil {
		return err
	}

	out := results{Settings: currentSettings(options.suite, *runs, provider, *tags), Base: base, Head: head}
	if options.corpora != "" {
		out.Settings.Suite = options.corpora
	}
	builds := []build{base, head}
	leaked := false
	for _, entry := range selected {
		for index, query := range entry.Queries {
			item := scenario{
				Name:   fmt.Sprintf("BM_Search/%s/q%d", entry.Name, index+1),
				Corpus: entry.Name, Query: query, Runs: map[string][]sample{},
			}
			arguments := []string{query, dirs[entry.Name]}
			for _, b := range builds {
				if _, err := runSearch(b.Binary, env, arguments, fake, entry.MustExclude); err != nil {
					return fmt.Errorf("%s warm-up with %s: %w", item.Name, b.Label, err)
				}
			}
			for run := 0; run < *runs; run++ {
				for _, b := range builds {
					result, err := runSearch(b.Binary, env, arguments, fake, entry.MustExclude)
					if err != nil {
						return fmt.Errorf("%s with %s: %w", item.Name, b.Label, err)
					}
					item.Runs[b.Label] = append(item.Runs[b.Label], result)
					if b.Label == "head" && result.Provider.LeakedBodies > 0 {
						leaked = true
					}
				}
			}
			headWall, _, last := item.medians("head")
			baseWall, _, _ := item.medians("base")
			logf("%s: base %.0fms, head %.0fms, %d requests, output %s", item.Name, baseWall, headWall, last.Provider.Requests, item.outputs())
			out.Scenarios = append(out.Scenarios, item)
		}
	}
	if *detector {
		roots := make([]string, 0, len(selected))
		for _, entry := range selected {
			roots = append(roots, dirs[entry.Name])
		}
		comparison := &detectComparison{}
		for _, b := range builds {
			binary, err := buildDetector(b, options.cache, *tags, scratch, logf)
			if err != nil {
				return err
			}
			logf("running detector benchmark for %s", b.Label)
			result, err := runDetector(binary, roots)
			if err != nil {
				return err
			}
			if b.Label == "base" {
				comparison.Base = result
			} else {
				comparison.Head = result
			}
		}
		out.Detect = comparison
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	report := out.markdown(*repoURL)
	if err := os.WriteFile(filepath.Join(*outDir, "report.md"), []byte(report), 0o644); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(filepath.Join(*outDir, "results.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Print(report)
	logf("wrote %s", filepath.Join(*outDir, "report.md"))
	if leaked {
		return errors.New("head sent must-exclude content to the provider")
	}
	return nil
}

// webURL turns the origin remote into a browsable URL for commit links.
func webURL(repo string) string {
	remote, err := git(repo, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	remote = strings.TrimSuffix(remote, ".git")
	if rest, ok := strings.CutPrefix(remote, "git@github.com:"); ok {
		return "https://github.com/" + rest
	}
	if strings.HasPrefix(remote, "https://") {
		return remote
	}
	return ""
}
