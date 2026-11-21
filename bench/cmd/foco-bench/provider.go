package main

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// providerOptions shape the fake System One provider. MaxQuestions mirrors
// Command Code's Jev endpoint, which rejects more than 100 questions per call.
// Jev's 32K-token context cannot be checked exactly without its tokenizer, so
// by default requests over WarnContext bytes of state plus longest question
// are only counted; MaxContext rejects them instead.
type providerOptions struct {
	Latency      time.Duration
	LatencyPerKB time.Duration
	MaxQuestions int
	MaxContext   int
	WarnContext  int
	TLS          bool
}

type providerStats struct {
	Requests     int64 `json:"requests"`
	Rejected     int64 `json:"rejected"`
	Connections  int64 `json:"connections"`
	MaxInFlight  int64 `json:"maxInFlight"`
	BytesIn      int64 `json:"bytesIn"`
	LeakedBodies int64 `json:"leakedBodies"`
	OverContext  int64 `json:"overContext"`
	// RejectedByKind counts rejected requests per request kind (navigation,
	// selection, assessment), and LargestContext is the largest state plus
	// longest question seen, to show how close requests come to the limit.
	RejectedByKind map[string]int64 `json:"rejectedByKind,omitempty"`
	LargestContext int64            `json:"largestContext"`
}

// fakeProvider answers deterministically: a score depends only on the query,
// the item path or the request state, and the question ID, never on batching,
// timing or order, so two foco builds that ask the same questions get the same
// answers.
type fakeProvider struct {
	options  providerOptions
	server   *httptest.Server
	caFile   string
	inFlight atomic.Int64
	mu       sync.Mutex
	stats    providerStats
	forbid   []string
}

func startProvider(options providerOptions, scratch string) (*fakeProvider, error) {
	provider := &fakeProvider{options: options}
	provider.server = httptest.NewUnstartedServer(http.HandlerFunc(provider.serve))
	provider.server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			provider.mu.Lock()
			provider.stats.Connections++
			provider.mu.Unlock()
		}
	}
	if options.TLS {
		provider.server.EnableHTTP2 = true
		provider.server.StartTLS()
		provider.caFile = scratch + "/provider-ca.crt"
		block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.server.Certificate().Raw})
		if err := os.WriteFile(provider.caFile, block, 0o644); err != nil {
			provider.server.Close()
			return nil, err
		}
	} else {
		provider.server.Start()
	}
	return provider, nil
}

func (p *fakeProvider) endpoint() string { return p.server.URL + "/v1" }

func (p *fakeProvider) close() { p.server.Close() }

// reset clears the counters and sets the strings no request body may contain.
func (p *fakeProvider) reset(forbid []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats = providerStats{}
	p.forbid = forbid
}

func (p *fakeProvider) snapshot() providerStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// splitQuestion splits IDs such as q3, scope3 or ref3 into prefix and index.
func splitQuestion(id string) (string, int) {
	cut := strings.IndexAny(id, "0123456789")
	if cut <= 0 {
		return id, -1
	}
	var index int
	if _, err := fmt.Sscanf(id[cut:], "%d", &index); err != nil {
		return id, -1
	}
	return id[:cut], index
}

func requestKind(state json.RawMessage) string {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(state, &fields)
	switch {
	case fields["items"] != nil:
		return "navigation"
	case fields["declarations"] != nil:
		return "selection"
	case fields["preview"] != nil:
		return "assessment"
	}
	return "other"
}

func score(key string) float64 {
	hash := fnv.New64a()
	hash.Write([]byte(key))
	return float64(hash.Sum64()%10000) / 10000
}

func (p *fakeProvider) serve(writer http.ResponseWriter, request *http.Request) {
	current := p.inFlight.Add(1)
	defer p.inFlight.Add(-1)
	raw, _ := io.ReadAll(request.Body)
	var body struct {
		State     json.RawMessage            `json:"state"`
		Questions map[string]json.RawMessage `json:"questions"`
	}
	_ = json.Unmarshal(raw, &body)
	longest := 0
	for _, question := range body.Questions {
		longest = max(longest, len(question))
	}
	p.mu.Lock()
	p.stats.Requests++
	p.stats.BytesIn += int64(len(raw))
	p.stats.MaxInFlight = max(p.stats.MaxInFlight, current)
	for _, forbidden := range p.forbid {
		if strings.Contains(string(raw), forbidden) {
			p.stats.LeakedBodies++
			break
		}
	}
	rejected := (p.options.MaxQuestions > 0 && len(body.Questions) > p.options.MaxQuestions) ||
		(p.options.MaxContext > 0 && len(body.State)+longest > p.options.MaxContext)
	context := int64(len(body.State) + longest)
	p.stats.LargestContext = max(p.stats.LargestContext, context)
	if p.options.WarnContext > 0 && context > int64(p.options.WarnContext) {
		p.stats.OverContext++
	}
	if rejected {
		p.stats.Rejected++
		if p.stats.RejectedByKind == nil {
			p.stats.RejectedByKind = map[string]int64{}
		}
		p.stats.RejectedByKind[requestKind(body.State)]++
	}
	p.mu.Unlock()
	time.Sleep(p.options.Latency + time.Duration(len(raw)/1024)*p.options.LatencyPerKB)
	if rejected {
		http.Error(writer, fmt.Sprintf("at most %d questions per call and %d context bytes", p.options.MaxQuestions, p.options.MaxContext), http.StatusBadRequest)
		return
	}
	var state struct {
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
		Path         string `json:"path"`
		Query        string `json:"query"`
		Declarations []struct {
			StartLine int `json:"startLine"`
			EndLine   int `json:"endLine"`
		} `json:"declarations"`
	}
	_ = json.Unmarshal(body.State, &state)
	answers := map[string]any{}
	for id := range body.Questions {
		var value float64
		prefix, index := splitQuestion(id)
		if state.Items != nil && prefix == "q" && index >= 0 && index < len(state.Items) {
			// About a quarter of navigation items are relevant.
			if x := score(state.Query + "|" + state.Items[index].Path); x < 0.25 {
				value = 0.55 + x
			} else {
				value = x * 0.5
			}
		} else if state.Declarations != nil && index >= 0 && index < len(state.Declarations) {
			// Score each source window by its own lines, so how windows are
			// grouped into requests never changes its answer.
			window := state.Declarations[index]
			value = 0.05 + 0.9*score(fmt.Sprintf("%s|%s|%d-%d|%s", state.Query, state.Path, window.StartLine, window.EndLine, prefix))
		} else {
			key := state.Path
			if key == "" {
				key = string(body.State)
			}
			value = 0.05 + 0.9*score(key+"|"+id)
		}
		answers[id] = map[string]any{"type": "noul", "noul": value}
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"answers": answers})
}
