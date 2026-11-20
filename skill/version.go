// Package skill ships the agent skill installed by "foco skill".
package skill

import (
	"embed"
	"fmt"
)

// Version is the current skill content version. Its directory name is part of
// the skill path.
const Version = "v0"

// Path is the path of the current skill content inside the embedded files.
const Path = Version + "/SKILL.md"

//go:embed v[0-9]*
var files embed.FS

// Content is the current skill.
var Content = mustRead(Path)

func mustRead(path string) string {
	data, err := files.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("foco skill: %v", err))
	}
	return string(data)
}
