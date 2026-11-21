//go:build vectorscan

package detect

import (
	"os"
	"sync"

	"github.com/praetorian-inc/titus/pkg/matcher"
)

// prefilterTitusRules is off: Vectorscan already scans all rules in one pass,
// and splitting rules across databases measured four times slower.
const prefilterTitusRules = false

var stderrMu sync.Mutex

// newTitusMatcher builds the Vectorscan matcher with os.Stderr pointed at the
// null device: Titus prints database-cache and compile diagnostics there
// unconditionally, which would leak into foco's own stderr on every search.
func newTitusMatcher(cfg matcher.Config) (matcher.Matcher, error) {
	stderrMu.Lock()
	defer stderrMu.Unlock()
	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		saved := os.Stderr
		os.Stderr = null
		defer func() {
			os.Stderr = saved
			_ = null.Close()
		}()
	}
	return matcher.New(cfg)
}
