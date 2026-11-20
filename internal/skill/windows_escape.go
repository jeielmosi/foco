package skill

import (
	"regexp"
	"strings"
)

var windowsCommandMetaCharacters = regexp.MustCompile("([()\\[\\]{}%!^\"`<>&|;, *?])")

func escapeWindowsCommand(command string) string {
	return windowsCommandMetaCharacters.ReplaceAllString(command, "^$1")
}

func escapeWindowsArgument(argument string) string {
	var escaped strings.Builder
	backslashes := 0
	for _, character := range argument {
		switch character {
		case '\\':
			backslashes++
		case '"':
			escaped.WriteString(strings.Repeat("\\", backslashes*2+1))
			escaped.WriteRune(character)
			backslashes = 0
		default:
			escaped.WriteString(strings.Repeat("\\", backslashes))
			backslashes = 0
			escaped.WriteRune(character)
		}
	}
	escaped.WriteString(strings.Repeat("\\", backslashes*2))
	quoted := `"` + escaped.String() + `"`
	return windowsCommandMetaCharacters.ReplaceAllString(quoted, "^$1")
}

func windowsInstallerCommand(args []string) []string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, escapeWindowsCommand("npx"))
	for _, argument := range args {
		parts = append(parts, escapeWindowsArgument(argument))
	}
	return []string{"/d", "/s", "/c", `"` + strings.Join(parts, " ") + `"`}
}
