package skill

import "testing"

func TestEscapeWindowsArgument(t *testing.T) {
	cases := map[string]string{
		"plain": `^"plain^"`,
		`a"b`:   `^"a\^"b^"`,
		`a\`:    `^"a\\^"`,
		`a\\`:   `^"a\\\\^"`,
		`a\b`:   `^"a\b^"`,
		`50%`:   `^"50^%^"`,
		`a&b`:   `^"a^&b^"`,
		`a b`:   `^"a^ b^"`,
	}
	for argument, want := range cases {
		if got := escapeWindowsArgument(argument); got != want {
			t.Fatalf("%q: got %q, want %q", argument, got, want)
		}
	}
}

func TestEscapeWindowsCommand(t *testing.T) {
	if got := escapeWindowsCommand("npx"); got != "npx" {
		t.Fatalf("npx: %q", got)
	}
	if got := escapeWindowsCommand("a&b"); got != "a^&b" {
		t.Fatalf("a&b: %q", got)
	}
}
