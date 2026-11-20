package catalog

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

//go:embed paths.yaml
var embedded []byte

type file struct {
	Names []string `yaml:"names"`
	Globs []string `yaml:"globs"`
}

type Catalog struct {
	names map[string]struct{}
	globs []string
}

var (
	once   sync.Once
	loaded *Catalog
	err    error
)

func Load() (*Catalog, error) {
	once.Do(func() {
		var raw file
		if e := yaml.Unmarshal(embedded, &raw); e != nil {
			err = e
			return
		}
		for _, glob := range raw.Globs {
			if !doublestar.ValidatePattern(glob) {
				err = fmt.Errorf("catalog: invalid glob pattern %q", glob)
				return
			}
		}
		c := &Catalog{names: map[string]struct{}{}, globs: raw.Globs}
		for _, name := range raw.Names {
			c.names[strings.ToLower(name)] = struct{}{}
		}
		loaded = c
	})
	return loaded, err
}

func (c *Catalog) Match(path string) bool {
	if c == nil {
		return false
	}
	path = strings.ToLower(filepath.ToSlash(path))
	name := strings.ToLower(filepath.Base(path))
	if _, ok := c.names[name]; ok {
		return true
	}
	for _, glob := range c.globs {
		if ok, _ := doublestar.Match(glob, path); ok {
			return true
		}
	}
	return false
}
