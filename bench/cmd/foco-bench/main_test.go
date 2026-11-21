package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
)

func TestManifestPinsEveryCorpus(t *testing.T) {
	loaded, err := loadManifest("../../corpora.json")
	if err != nil {
		t.Fatal(err)
	}
	hex := regexp.MustCompile(`^[0-9a-f]{40}$`)
	names := map[string]bool{}
	for _, entry := range loaded.Corpora {
		if !hex.MatchString(entry.Commit) {
			t.Errorf("%s: commit %q is not a full SHA", entry.Name, entry.Commit)
		}
		if names[entry.Name] {
			t.Errorf("duplicate corpus %s", entry.Name)
		}
		names[entry.Name] = true
	}
	small, err := selectCorpora(loaded.Corpora, "small", nil)
	if err != nil || len(small) == 0 {
		t.Fatalf("small suite: %d corpora, %v", len(small), err)
	}
	large, _ := selectCorpora(loaded.Corpora, "large", nil)
	if len(large) != len(loaded.Corpora) {
		t.Fatalf("large suite has %d of %d corpora", len(large), len(loaded.Corpora))
	}
}

func ask(t *testing.T, provider *fakeProvider, state any, questions []string) (int, map[string]float64) {
	t.Helper()
	asked := map[string]any{}
	for _, id := range questions {
		asked[id] = map[string]string{"type": "noul", "instructions": "x"}
	}
	body, _ := json.Marshal(map[string]any{"model": "m", "state": state, "questions": asked})
	response, err := http.Post(provider.endpoint(), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded struct {
		Answers map[string]struct {
			Noul float64 `json:"noul"`
		} `json:"answers"`
	}
	_ = json.NewDecoder(response.Body).Decode(&decoded)
	out := map[string]float64{}
	for id, answer := range decoded.Answers {
		out[id] = answer.Noul
	}
	return response.StatusCode, out
}

func TestFakeProviderScoresWindowsIndependentlyOfGrouping(t *testing.T) {
	provider, err := startProvider(providerOptions{MaxQuestions: 4}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.close()
	window := func(start int) map[string]int { return map[string]int{"startLine": start, "endLine": start + 39} }
	_, together := ask(t, provider, map[string]any{
		"query": "q", "path": "a.go", "declarations": []any{window(1), window(31)},
	}, []string{"q0", "scope0", "q1", "scope1"})
	_, alone := ask(t, provider, map[string]any{
		"query": "q", "path": "a.go", "declarations": []any{window(31)},
	}, []string{"q0", "scope0"})
	if together["q1"] != alone["q0"] || together["scope1"] != alone["scope0"] {
		t.Fatalf("window 31 scored %v together and %v alone", together, alone)
	}
	status, _ := ask(t, provider, map[string]any{"query": "q"}, []string{"a", "b", "c", "d", "e"})
	if status != http.StatusBadRequest || provider.snapshot().Rejected != 1 {
		t.Fatalf("status = %d, stats = %+v", status, provider.snapshot())
	}
}
