//go:build !vectorscan

package detect

// prefilterTitusRules is on for the portable regexp2 matcher, which otherwise
// runs every rule over every file. The Vectorscan matcher already scans all
// rules in one pass and keeps a single database.
const prefilterTitusRules = true
