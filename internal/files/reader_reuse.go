package files

import (
	"os"
	"time"
)

const reusableEntryOverhead int64 = 256

type fileIdentity struct {
	info    os.FileInfo
	size    int64
	mtimeNs int64
	ctimeNs int64
}

type reusableSnapshot struct {
	snapshot Snapshot
	identity fileIdentity
	bytes    int64
}

func identityFor(info os.FileInfo) fileIdentity {
	identity := fileIdentity{info: info, size: info.Size(), mtimeNs: info.ModTime().UnixNano()}
	if change, ok := changeTime(info); ok {
		identity.ctimeNs = change
	}
	return identity
}

func canReuseIdentity(identity fileIdentity, startedNs int64) bool {
	last := identity.mtimeNs
	if identity.ctimeNs > last {
		last = identity.ctimeNs
	}
	return last+int64(3*time.Second) < startedNs
}

func sameIdentityAt(path string, identity fileIdentity) bool {
	current, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if !sameFileInfo(identity.info, current) ||
		current.Size() != identity.size ||
		current.ModTime().UnixNano() != identity.mtimeNs {
		return false
	}
	if change, ok := changeTime(current); ok && change != identity.ctimeNs {
		return false
	}
	return true
}

func (r *Reader) forgetReusable(path string) {
	r.reuseMu.Lock()
	defer r.reuseMu.Unlock()
	r.forgetReusableLocked(path)
}

func (r *Reader) forgetReusableLocked(path string) {
	entry, ok := r.reusable[path]
	if !ok {
		return
	}
	delete(r.reusable, path)
	r.reusableBytes -= entry.bytes
	for index, key := range r.reuseOrder {
		if key == path {
			r.reuseOrder = append(r.reuseOrder[:index], r.reuseOrder[index+1:]...)
			break
		}
	}
}

func (r *Reader) touchReusableLocked(path string) {
	for index, key := range r.reuseOrder {
		if key == path {
			r.reuseOrder = append(r.reuseOrder[:index], r.reuseOrder[index+1:]...)
			break
		}
	}
	r.reuseOrder = append(r.reuseOrder, path)
}

func (r *Reader) retainReusable(path string, snapshot Snapshot, identity fileIdentity, size int64) {
	r.reuseMu.Lock()
	defer r.reuseMu.Unlock()
	r.forgetReusableLocked(path)
	cost := reusableEntryOverhead + size
	if cost > r.limits.MaxReusableBytes {
		return
	}
	r.reusable[path] = reusableSnapshot{snapshot: snapshot, identity: identity, bytes: cost}
	r.reuseOrder = append(r.reuseOrder, path)
	r.reusableBytes += cost
	for r.reusableBytes > r.limits.MaxReusableBytes && len(r.reuseOrder) > 0 {
		oldest := r.reuseOrder[0]
		entry := r.reusable[oldest]
		delete(r.reusable, oldest)
		r.reuseOrder = r.reuseOrder[1:]
		r.reusableBytes -= entry.bytes
	}
}

func (r *Reader) reusableSnapshot(path string) (Snapshot, bool) {
	r.reuseMu.Lock()
	defer r.reuseMu.Unlock()
	entry, ok := r.reusable[path]
	if !ok {
		return Snapshot{}, false
	}
	if !sameIdentityAt(path, entry.identity) {
		r.forgetReusableLocked(path)
		return Snapshot{}, false
	}
	r.touchReusableLocked(path)
	return entry.snapshot, true
}
