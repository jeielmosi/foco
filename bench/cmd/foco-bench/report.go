package main

import (
	"fmt"
	"runtime"
	"strings"
)

type scenario struct {
	Name   string              `json:"name"`
	Corpus string              `json:"corpus"`
	Query  string              `json:"query"`
	Runs   map[string][]sample `json:"runs"`
}

type detectComparison struct {
	Base detectResult `json:"base"`
	Head detectResult `json:"head"`
}

type settings struct {
	Suite        string `json:"suite"`
	Runs         int    `json:"runs"`
	Latency      string `json:"latency"`
	LatencyPerKB string `json:"latencyPerKb"`
	MaxQuestions int    `json:"maxQuestions"`
	MaxContext   int    `json:"maxContext"`
	WarnContext  int    `json:"warnContext"`
	TLS          bool   `json:"tls"`
	Tags         string `json:"tags"`
	CPUs         int    `json:"cpus"`
	Platform     string `json:"platform"`
	GoVersion    string `json:"goVersion"`
}

type results struct {
	Settings  settings          `json:"settings"`
	Base      build             `json:"base"`
	Head      build             `json:"head"`
	Scenarios []scenario        `json:"scenarios"`
	Detect    *detectComparison `json:"detect,omitempty"`
}

func (s scenario) medians(label string) (wall, user float64, last sample) {
	runs := s.Runs[label]
	walls, users := make([]float64, len(runs)), make([]float64, len(runs))
	for index, run := range runs {
		walls[index], users[index] = run.WallMS, run.UserMS
	}
	return median(walls), median(users), runs[len(runs)-1]
}

// outputs reports whether every run of both builds printed the same bytes,
// and whether each build was at least consistent with itself.
func (s scenario) outputs() string {
	hashes := map[string]map[string]bool{"base": {}, "head": {}}
	for label, runs := range s.Runs {
		for _, run := range runs {
			hashes[label][run.OutputHash] = true
		}
	}
	switch {
	case len(hashes["base"]) == 1 && len(hashes["head"]) == 1:
		for hash := range hashes["base"] {
			if hashes["head"][hash] {
				return "identical"
			}
		}
		return "differs"
	case len(hashes["head"]) > 1:
		return "head varies between runs"
	default:
		return "base varies between runs"
	}
}

func relative(old, new float64) string {
	if old == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%+.4f", (new-old)/old)
}

func (b build) link(repoURL string) string {
	if b.Commit == "" {
		return "`" + b.Ref + "`"
	}
	return commitLink(repoURL, b.Commit)
}

func commitLink(repoURL, commit string) string {
	if repoURL == "" {
		return "`" + commit[:8] + "`"
	}
	return fmt.Sprintf("[`%s`](%s/commit/%s)", commit[:8], repoURL, commit)
}

func (r results) markdown(repoURL string) string {
	var out strings.Builder
	s := r.Settings
	engine := "pure Go"
	if strings.Contains(s.Tags, "vectorscan") {
		engine = "Vectorscan"
	}
	transport := "HTTP"
	if s.TLS {
		transport = "TLS"
	}
	limits := []string{}
	if s.MaxQuestions > 0 {
		limits = append(limits, fmt.Sprintf("rejecting requests with more than %d questions (HTTP 400, as Jev does)", s.MaxQuestions))
	}
	if s.MaxContext > 0 {
		limits = append(limits, fmt.Sprintf("rejecting requests over %d bytes of state plus longest question", s.MaxContext))
	}
	if s.WarnContext > 0 {
		limits = append(limits, fmt.Sprintf("counting requests over %d bytes of state plus longest question, which risk Jev's 32K-token context", s.WarnContext))
	}
	if len(limits) == 0 {
		limits = append(limits, "no request limits")
	}
	latency := s.Latency + " per response"
	if s.LatencyPerKB != "0s" {
		latency += " plus " + s.LatencyPerKB + " per KB sent"
	}
	fmt.Fprintf(&out, "### Benchmark\n\n")
	fmt.Fprintf(&out, "Scenario: `foco \"<query>\" <corpus>` end to end on the `%s` suite of [`bench/corpora.json`](bench/corpora.json) (each corpus pinned to a commit), against the deterministic fake System One provider in `bench/cmd/foco-bench` over %s with %s, %s. %s build, %d CPUs, %s, %s. Time is wall clock and CPU is user time, each the median of %d run(s) after one warm-up run per build.\n\n",
		s.Suite, transport, latency, strings.Join(limits, ", "), engine, s.CPUs, s.Platform, s.GoVersion, s.Runs)
	fmt.Fprintf(&out, "Benchmark comparison for %s (base) vs %s (PR)\n\n", r.Base.link(repoURL), r.Head.link(repoURL))
	out.WriteString("| Benchmark | Time | CPU | Time Old | Time New | CPU Old | CPU New |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, item := range r.Scenarios {
		baseWall, baseUser, _ := item.medians("base")
		headWall, headUser, _ := item.medians("head")
		fmt.Fprintf(&out, "| `%s` | %s | %s | %.0fms | %.0fms | %.0fms | %.0fms |\n",
			item.Name, relative(baseWall, headWall), relative(baseUser, headUser), baseWall, headWall, baseUser, headUser)
	}
	out.WriteString("\nScenario: provider traffic and output per search, counted by the fake provider. Rejected requests got HTTP 400 for exceeding a provider limit; oversized requests passed but risk a real model's context limit; leaked requests carried content the corpus marks as must-exclude.\n\n")
	out.WriteString("| Benchmark | Requests Old | Requests New | Rejected Old | Rejected New | Oversized Old | Oversized New | Connections Old | Connections New | Leaked Old | Leaked New | Output |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, item := range r.Scenarios {
		_, _, base := item.medians("base")
		_, _, head := item.medians("head")
		fmt.Fprintf(&out, "| `%s` | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %s |\n", item.Name,
			base.Provider.Requests, head.Provider.Requests, base.Provider.Rejected, head.Provider.Rejected,
			base.Provider.OverContext, head.Provider.OverContext, base.Provider.Connections, head.Provider.Connections, base.Provider.LeakedBodies, head.Provider.LeakedBodies, item.outputs())
	}
	if r.Detect != nil {
		base, head := r.Detect.Base, r.Detect.Head
		fmt.Fprintf(&out, "\nScenario: the secret detector alone (Titus + Betterleaks union) over every text file up to 1 MB in the suite (%d files), with one caller per CPU. Base excluded %d files and head %d; %s.\n\n",
			head.Files, len(base.Excluded), len(head.Excluded), verdictSummary(base.Excluded, head.Excluded))
		out.WriteString("| Benchmark | Time | CPU | Time Old | Time New | CPU Old | CPU New |\n|---|---:|---:|---:|---:|---:|---:|\n")
		fmt.Fprintf(&out, "| `BM_Detect/%s` | %s | %s | %.0fms | %.0fms | %.0fms | %.0fms |\n", s.Suite,
			relative(base.WallMS, head.WallMS), relative(base.UserMS, head.UserMS), base.WallMS, head.WallMS, base.UserMS, head.UserMS)
		fmt.Fprintf(&out, "| `BM_Detect/%s/init` | %s | | %dms | %dms | | |\n", s.Suite,
			relative(float64(base.InitMS), float64(head.InitMS)), base.InitMS, head.InitMS)
	}
	return out.String()
}

func verdictSummary(base, head []string) string {
	inBase := map[string]bool{}
	for _, path := range base {
		inBase[path] = true
	}
	inHead := map[string]bool{}
	for _, path := range head {
		inHead[path] = true
	}
	missed, added := 0, 0
	for path := range inBase {
		if !inHead[path] {
			missed++
		}
	}
	for path := range inHead {
		if !inBase[path] {
			added++
		}
	}
	if missed == 0 && added == 0 {
		return "per-file verdicts matched on every file"
	}
	return fmt.Sprintf("**verdicts differ: head no longer excludes %d files and newly excludes %d** (listed in results.json)", missed, added)
}

func currentSettings(suite string, runs int, options providerOptions, tags string) settings {
	return settings{
		Suite: suite, Runs: runs, Latency: options.Latency.String(), LatencyPerKB: options.LatencyPerKB.String(),
		MaxQuestions: options.MaxQuestions, MaxContext: options.MaxContext, WarnContext: options.WarnContext, TLS: options.TLS, Tags: tags,
		CPUs: runtime.NumCPU(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(),
	}
}
