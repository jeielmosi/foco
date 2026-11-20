package filetypes

import (
	"path/filepath"
	"regexp"
	"strings"

	"foco/internal/enums"
)

var ordered = []enums.FileCategory{
	enums.FileCategoryCode, enums.FileCategoryConfig, enums.FileCategoryDocs,
	enums.FileCategoryData, enums.FileCategoryAudio, enums.FileCategoryVideo,
	enums.FileCategoryArchives, enums.FileCategoryLogs, enums.FileCategoryTemp,
	enums.FileCategoryUnknown,
}

// All returns the category names in the same order as upstream fileTypes.
func All() []enums.FileCategory {
	return append([]enums.FileCategory(nil), ordered...)
}

var byExtension = map[string]enums.FileCategory{}

func init() {
	add := func(c enums.FileCategory, exts string) {
		for _, ext := range strings.Fields(exts) {
			byExtension[ext] = c
		}
	}
	add(enums.FileCategoryCode, "c h cc hh cpp hpp cxx hxx cs fs fsx vb java kt kts scala clj cljs cljc edn groovy go rs py pyw pyi pyx rb php pl pm r R jl lua sh bash zsh fish ps1 psm1 bat cmd js jsx mjs cjs ts tsx mts cts vue svelte astro html htm css scss sass less sql swift m mm dart ex exs erl hrl hs lhs ml mli elm f f90 f95 for pas asm s v vhd vhdl zig nim sol tf tfvars proto graphql gql cmake")
	add(enums.FileCategoryConfig, "yaml yml toml ini cfg conf config properties env editorconfig")
	add(enums.FileCategoryDocs, "md mdx markdown rst txt adoc asciidoc tex org pod")
	add(enums.FileCategoryData, "json jsonl ndjson csv tsv xml arff parquet arrow feather avro orc h5 hdf5 hdf npy npz mat pkl pickle sqlite sqlite3 db sqlitedb ipynb geojson kml gpx pstats")
	add(enums.FileCategoryAudio, "mp3 wav flac aac ogg oga opus m4a aiff aif wma mid midi")
	add(enums.FileCategoryVideo, "mp4 m4v mkv mov avi webm mpg mpeg m2ts vob flv wmv 3gp")
	add(enums.FileCategoryArchives, "zip gz bz2 xz zst zstd tar tgz tbz tbz2 txz 7z rar jar war whl deb rpm iso dmg cab")
	add(enums.FileCategoryLogs, "log out err trace")
	add(enums.FileCategoryTemp, "tmp temp bak swp swo orig rej")
}

var codeNames = map[string]struct{}{
	"makefile": {}, "gnumakefile": {}, "dockerfile": {}, "containerfile": {}, "rakefile": {},
	"gemfile": {}, "justfile": {}, "jenkinsfile": {}, "vagrantfile": {}, "build": {},
	"workspace": {}, "cmakelists.txt": {},
}

var configNames = map[string]struct{}{
	".editorconfig": {}, ".env": {}, ".gitignore": {}, ".ignore": {}, ".gitattributes": {},
	".dockerignore": {}, ".npmrc": {}, ".babelrc": {}, ".eslintrc": {}, ".prettierrc": {},
	"package.json": {}, "package-lock.json": {}, "yarn.lock": {}, "bun.lock": {},
	"cargo.lock": {}, "tsconfig.json": {}, "jsconfig.json": {}, "composer.json": {},
	"deno.json": {}, "deno.jsonc": {},
}

var tempRE = regexp.MustCompile(`^#.*#$`)
var logRE = regexp.MustCompile(`\.log(?:\.\d+)?(?:\.(?:gz|bz2|xz|zst))?$`)

func Classify(path string, content []byte) (enums.FileCategory, string) {
	name := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(name, "~") || tempRE.MatchString(name) {
		return enums.FileCategoryTemp, ""
	}
	if logRE.MatchString(name) {
		return enums.FileCategoryLogs, ""
	}
	if _, ok := codeNames[name]; ok || strings.HasPrefix(name, "dockerfile.") {
		return enums.FileCategoryCode, ""
	}
	if _, ok := configNames[name]; ok || strings.HasPrefix(name, ".env.") || isJSONConfig(name) {
		return enums.FileCategoryConfig, ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if c, ok := byExtension[ext]; ok {
		return c, ""
	}
	if ext == "" {
		if lang := shebangLanguage(content); lang != "" {
			return enums.FileCategoryCode, lang
		}
	}
	return enums.FileCategoryUnknown, ""
}

func isJSONConfig(name string) bool {
	if strings.HasPrefix(name, ".") && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonc")) {
		return true
	}
	for _, prefix := range []string{"biome.json", "tsconfig", "jsconfig"} {
		if strings.HasPrefix(name, prefix) && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonc")) {
			return true
		}
	}
	return strings.HasSuffix(name, ".config.json") || strings.HasSuffix(name, ".config.jsonc")
}

func shebangLanguage(content []byte) string {
	first := content
	if i := strings.IndexByte(string(content), '\n'); i >= 0 {
		first = content[:i]
	}
	line := strings.TrimSpace(string(first))
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	for i := 0; i < len(fields); i++ {
		base := filepath.Base(fields[i])
		switch base {
		case "env":
			for j := i + 1; j < len(fields); j++ {
				if strings.HasPrefix(fields[j], "-") || strings.Contains(fields[j], "=") {
					continue
				}
				return interpreter(fields[j])
			}
		case "python", "python3", "pypy", "pypy3":
			return "python"
		case "node", "nodejs":
			return "javascript"
		case "bun", "tsx", "deno":
			return "typescript"
		}
	}
	return ""
}

func interpreter(name string) string {
	switch name {
	case "python", "python3", "pypy", "pypy3":
		return "python"
	case "node", "nodejs":
		return "javascript"
	case "bun", "tsx", "deno":
		return "typescript"
	}
	return ""
}
