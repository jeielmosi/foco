package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"foco/internal/enums"
)

const pageSize = 128

type ContentChecker interface {
	DetectPath(ctx context.Context, path string) (bool, error)
}

type Snapshot struct {
	Path        string
	Source      string
	ContentHash string
}

type Decision struct {
	Path     string
	Eligible bool
	Reason   enums.FileReason
	Info     os.FileInfo
}

type Entry struct {
	Path  string
	Name  string
	IsDir bool
	Bytes int64
}

type Page struct {
	Path       string
	Entries    []Entry
	Excluded   map[string]int
	NextOffset int
	Issue      enums.FileIssueKind
}

type LookupResult struct {
	Status enums.LookupStatus
	Reason enums.FileReason
}

type SnapshotResult struct {
	Status   enums.SnapshotStatus
	Snapshot *Snapshot
	Reason   enums.FileReason
	Issue    enums.FileIssueKind
}

type Reader struct {
	root          string
	policy        Policy
	detector      ContentChecker
	protected     []string
	ignore        *IgnoreMatcher
	limits        Limits
	now           func() time.Time
	openDirs      chan struct{}
	reuseMu       sync.Mutex
	reusable      map[string]reusableSnapshot
	reuseOrder    []string
	reusableBytes int64
}

type Limits struct {
	MaxOpenDirectories int
	MaxReusableBytes   int64
	MaxIgnoreBytes     int64
}

func defaultLimits() Limits {
	return Limits{
		MaxOpenDirectories: 64,
		MaxReusableBytes:   32 << 20,
		MaxIgnoreBytes:     1 << 20,
	}
}

func normalizeLimits(limits Limits) Limits {
	defaults := defaultLimits()
	if limits.MaxOpenDirectories < 1 {
		limits.MaxOpenDirectories = defaults.MaxOpenDirectories
	}
	if limits.MaxReusableBytes < 1 {
		limits.MaxReusableBytes = defaults.MaxReusableBytes
	}
	if limits.MaxIgnoreBytes < 1 {
		limits.MaxIgnoreBytes = defaults.MaxIgnoreBytes
	}
	return limits
}

func New(root string, policy Policy, detector ContentChecker, protected []string) (*Reader, error) {
	return NewWithLimits(root, policy, detector, protected, defaultLimits())
}

func NewWithLimits(root string, policy Policy, detector ContentChecker, protected []string, limits Limits) (*Reader, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "open", Path: real, Err: syscall.ENOTDIR}
	}
	limits = normalizeLimits(limits)
	ignore, err := NewIgnoreMatcher(real, policy.NoIgnore, policy.Exclude)
	if err != nil {
		return nil, err
	}
	ignore.maxBytes = limits.MaxIgnoreBytes
	protected = append(append([]string{}, protected...), storageDir("config"), storageDir("cache"))
	return &Reader{
		root: real, policy: policy, detector: detector, protected: cleanProtected(protected),
		ignore: ignore, limits: limits, now: time.Now,
		openDirs: make(chan struct{}, limits.MaxOpenDirectories),
		reusable: map[string]reusableSnapshot{},
	}, nil
}

func (r *Reader) Close() error { return nil }

func (r *Reader) Root() string { return r.root }

func (r *Reader) Eligibility(ctx context.Context, path string) (Decision, error) {
	return r.decide(ctx, path, true, false)
}

func (r *Reader) LookupFile(ctx context.Context, path string) (LookupResult, error) {
	decision, err := r.decide(ctx, path, true, true)
	if err != nil {
		return LookupResult{}, err
	}
	switch {
	case decision.Eligible && decision.Info != nil && decision.Info.IsDir():
		return LookupResult{Status: enums.LookupStatusExcluded, Reason: enums.FileReasonNotFile}, nil
	case decision.Eligible:
		return LookupResult{Status: enums.LookupStatusFile}, nil
	case decision.Reason == enums.FileReasonMissing:
		return LookupResult{Status: enums.LookupStatusExcluded, Reason: enums.FileReasonMissing}, nil
	default:
		return LookupResult{Status: enums.LookupStatusExcluded, Reason: decision.Reason}, nil
	}
}

func (r *Reader) ListPage(ctx context.Context, path string, offset int) (Page, error) {
	page := Page{Path: path, Entries: []Entry{}, Excluded: map[string]int{}}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	select {
	case r.openDirs <- struct{}{}:
		defer func() { <-r.openDirs }()
	default:
		page.Issue = enums.FileIssueResourceLimit
		return page, nil
	}
	decision, err := r.decide(ctx, path, false, false)
	if err != nil {
		return page, err
	}
	page.Path = decision.Path
	if !decision.Eligible {
		if decision.Reason != enums.UnknownFileReason {
			page.Excluded[decision.Reason.String()] = 1
		}
		return page, nil
	}
	entries, err := os.ReadDir(decision.Path)
	if err != nil {
		return page, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	if offset < 0 {
		offset = 0
	}
	i := offset
	for ; i < len(entries) && len(page.Entries) < pageSize; i++ {
		child := filepath.Join(decision.Path, entries[i].Name())
		childDecision, err := r.decide(ctx, child, true, false)
		if err != nil {
			return page, err
		}
		if !childDecision.Eligible {
			if childDecision.Reason != enums.UnknownFileReason {
				page.Excluded[childDecision.Reason.String()]++
			}
			continue
		}
		info := childDecision.Info
		page.Entries = append(page.Entries, Entry{
			Path: childDecision.Path, Name: entries[i].Name(), IsDir: info.IsDir(), Bytes: info.Size(),
		})
	}
	if i < len(entries) {
		page.NextOffset = i
	}
	return page, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func storageDir(kind string) string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	if kind == "cache" {
		base, err = os.UserCacheDir()
		if err != nil {
			return ""
		}
	}
	return filepath.Join(base, "foco")
}

func cleanProtected(values []string) []string {
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if real, err := filepath.EvalSymlinks(value); err == nil {
			out = append(out, real)
		} else if abs, err := filepath.Abs(value); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func inside(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
