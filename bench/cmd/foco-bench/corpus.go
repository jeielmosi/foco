package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

type corpus struct {
	Name        string   `json:"name"`
	Suite       string   `json:"suite"`
	Git         string   `json:"git,omitempty"`
	File        string   `json:"file,omitempty"`
	Commit      string   `json:"commit"`
	Queries     []string `json:"queries"`
	MustExclude []string `json:"mustExclude,omitempty"`
}

type manifest struct {
	Corpora []corpus `json:"corpora"`
}

var suiteRank = map[string]int{"small": 0, "medium": 1, "large": 2}

func loadManifest(file string) (manifest, error) {
	var out manifest
	data, err := os.ReadFile(file)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("%s: %w", file, err)
	}
	for _, entry := range out.Corpora {
		if _, ok := suiteRank[entry.Suite]; !ok {
			return out, fmt.Errorf("%s: unknown suite %q", entry.Name, entry.Suite)
		}
		if (entry.Git == "") == (entry.File == "") || len(entry.Commit) != 40 || len(entry.Queries) == 0 {
			return out, fmt.Errorf("%s: needs exactly one of git or file, a 40-character commit and queries", entry.Name)
		}
	}
	return out, nil
}

// selectCorpora returns the corpora in suite (each suite includes the smaller
// ones), or exactly the named ones when names is set.
func selectCorpora(all []corpus, suite string, names []string) ([]corpus, error) {
	if len(names) > 0 {
		byName := map[string]corpus{}
		for _, entry := range all {
			byName[entry.Name] = entry
		}
		out := []corpus{}
		for _, name := range names {
			entry, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("unknown corpus %q", name)
			}
			out = append(out, entry)
		}
		return out, nil
	}
	rank, ok := suiteRank[suite]
	if !ok {
		return nil, fmt.Errorf("unknown suite %q (small, medium, large)", suite)
	}
	out := []corpus{}
	for _, entry := range all {
		if suiteRank[entry.Suite] <= rank {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (c corpus) dir(cache string) string {
	return filepath.Join(cache, "corpora", c.Name+"-"+c.Commit[:12])
}

// fetch materializes the corpus at its pinned commit. A completed fetch is
// marked by a sibling file, so the marker never becomes part of the corpus.
func (c corpus) fetch(cache string, logf func(string, ...any)) (string, error) {
	dir := c.dir(cache)
	done := dir + ".ok"
	if _, err := os.Stat(done); err == nil {
		return dir, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if c.File != "" {
		logf("fetching %s", c.File)
		if err := download(c.File, filepath.Join(dir, path.Base(c.File))); err != nil {
			return "", err
		}
	} else {
		logf("fetching %s at %s", c.Git, c.Commit[:12])
		steps := [][]string{
			{"init", "-q"},
			{"fetch", "-q", "--depth", "1", c.Git, c.Commit},
			{"-c", "advice.detachedHead=false", "checkout", "-q", "FETCH_HEAD"},
		}
		for _, step := range steps {
			command := exec.Command("git", step...)
			command.Dir = dir
			command.Stderr = os.Stderr
			if err := command.Run(); err != nil {
				return "", fmt.Errorf("%s: git %s: %w", c.Name, strings.Join(step, " "), err)
			}
		}
	}
	if err := os.WriteFile(done, []byte(c.Commit+"\n"), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func download(url, target string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, response.Status)
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
