package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// detectProgram drives a build's secret detector directly. It is compiled
// into each build through a go build overlay, so no file is written to the
// tree under test, and it only uses detector APIs present since the first
// release: NewTitus, NewBetterleaks, NewUnion, catalog.Load and DetectPath.
const detectProgram = `package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"foco/internal/detect"
	"foco/internal/detect/catalog"
)

func main() {
	type job struct{ label, path string }
	var jobs []job
	for _, root := range os.Args[1:] {
		filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
				return nil
			}
			relative, _ := filepath.Rel(filepath.Dir(root), path)
			jobs = append(jobs, job{relative, path})
			return nil
		})
	}
	started := time.Now()
	catalogValue, err := catalog.Load()
	if err != nil {
		panic(err)
	}
	titus, err := detect.NewTitus()
	if err != nil {
		panic(err)
	}
	better, err := detect.NewBetterleaks(min(8, runtime.GOMAXPROCS(0)))
	if err != nil {
		panic(err)
	}
	union := detect.NewUnion(titus, better, catalogValue)
	initialized := time.Since(started)
	var mu sync.Mutex
	excluded := []string{}
	errors := 0
	next := make(chan job)
	var wait sync.WaitGroup
	scanStarted := time.Now()
	for range runtime.GOMAXPROCS(0) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for item := range next {
				hit, err := union.DetectPath(context.Background(), item.path)
				mu.Lock()
				if err != nil {
					errors++
				} else if hit {
					excluded = append(excluded, item.label)
				}
				mu.Unlock()
			}
		}()
	}
	for _, item := range jobs {
		next <- item
	}
	close(next)
	wait.Wait()
	sort.Strings(excluded)
	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"files": len(jobs), "errors": errors, "excluded": excluded,
		"initMs": initialized.Milliseconds(), "scanMs": time.Since(scanStarted).Milliseconds(),
	})
}
`

type detectResult struct {
	Files    int      `json:"files"`
	Errors   int      `json:"errors"`
	Excluded []string `json:"excluded"`
	InitMS   int64    `json:"initMs"`
	ScanMS   int64    `json:"scanMs"`
	WallMS   float64  `json:"wallMs"`
	UserMS   float64  `json:"userMs"`
	MaxRSSKB int64    `json:"maxRssKb"`
}

func buildDetector(b build, cache, tags, scratch string, logf func(string, ...any)) (string, error) {
	source := filepath.Join(scratch, "detect-"+b.Commit[:12]+".go")
	if err := os.WriteFile(source, []byte(detectProgram), 0o644); err != nil {
		return "", err
	}
	virtual := filepath.Join(b.Dir, "internal", "focobenchdetect", "main.go")
	overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{virtual: source}})
	overlayFile := filepath.Join(scratch, "overlay-"+b.Commit[:12]+".json")
	if err := os.WriteFile(overlayFile, overlay, 0o644); err != nil {
		return "", err
	}
	binary := filepath.Join(cache, "bin", "detect-"+filepath.Base(b.Binary))
	logf("building detector benchmark for %s", b.Label)
	args := []string{"build", "-overlay", overlayFile, "-o", binary}
	env := append(os.Environ(), "CGO_ENABLED=0")
	if tags != "" {
		args = append(args, "-tags", tags)
		env = append(os.Environ(), "CGO_ENABLED=1")
	}
	command := exec.Command("go", append(args, "./internal/focobenchdetect")...)
	command.Dir = b.Dir
	command.Env = env
	command.Stderr = os.Stderr
	return binary, command.Run()
}

func runDetector(binary string, roots []string) (detectResult, error) {
	var out detectResult
	var stdout bytes.Buffer
	command := exec.Command(binary, roots...)
	command.Stdout = &stdout
	started := time.Now()
	if err := command.Run(); err != nil {
		return out, fmt.Errorf("detector benchmark: %w", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, err
	}
	out.WallMS = float64(time.Since(started).Microseconds()) / 1000
	out.UserMS, out.MaxRSSKB = usage(command.ProcessState)
	return out, nil
}
