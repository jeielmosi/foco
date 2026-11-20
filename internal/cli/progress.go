package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"foco/internal/retrieve"
	"foco/internal/tty"
)

var progressFrames = []string{"-", "\\", "|", "/"}

func progressSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KiB", "MiB", "GiB"}
	value := float64(bytes) / 1024
	unit := 0
	for unit < len(units)-1 && value >= 1024 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func progressLine(frame, files, bytes int) string {
	noun := "files"
	if files == 1 {
		noun = "file"
	}
	return fmt.Sprintf("%s Searching — %d %s · %s", progressFrames[frame%len(progressFrames)], files, noun, progressSize(bytes))
}

type Progress struct {
	mu      sync.Mutex
	enabled bool
	active  bool
	writer  io.Writer
	frame   int
	files   int
	bytes   int
	last    time.Time
	stop    chan struct{}
}

func newProgress(enabled bool, writer io.Writer) *Progress {
	return &Progress{enabled: enabled, writer: writer, stop: make(chan struct{})}
}

func (p *Progress) Start() {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	if p.active {
		p.mu.Unlock()
		return
	}
	p.active = true
	p.renderLocked(true)
	p.mu.Unlock()
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.mu.Lock()
				if p.active {
					p.frame = (p.frame + 1) % len(progressFrames)
					p.renderLocked(true)
				}
				p.mu.Unlock()
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *Progress) Update(snapshot retrieve.ProgressSnapshot) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	p.files, p.bytes = snapshot.Files, snapshot.Bytes
	p.renderLocked(false)
	p.mu.Unlock()
}

func (p *Progress) Done() {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return
	}
	p.active = false
	close(p.stop)
	fmt.Fprint(p.writer, "\r\033[2K")
}

func (p *Progress) renderLocked(force bool) {
	if !p.active || p.writer == nil {
		return
	}
	now := time.Now()
	if !force && now.Sub(p.last) < 100*time.Millisecond {
		return
	}
	p.last = now
	fmt.Fprintf(p.writer, "\r\033[2K%s", progressLine(p.frame, p.files, p.bytes))
}

func terminalWriter(writer io.Writer) bool {
	return tty.IsTerminal(writer)
}
