package cli

import (
	"bytes"
	"strings"
	"testing"

	"foco/internal/version"
)

func TestParityUsageAndExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"--help", "x"}, "Use --help or --version alone.\n", 1},
		{[]string{"--version", "x"}, "Use --help or --version alone.\n", 1},
		{[]string{"auth", "--stdin"}, "Use auth --schema system-one|gemini-decisions|openai-decisions --endpoint URL --model ID --stdin for a piped key.\n", 1},
		{[]string{"auth", "--schema", "unknown", "--endpoint", "https://x/decisions", "--model", "m", "--stdin"}, "Use auth --schema system-one|gemini-decisions|openai-decisions --endpoint URL --model ID --stdin for a piped key.\n", 1},
		{[]string{"auth", "--schema", "system-one", "--stdin"}, "All schemas need --endpoint URL and --model ID with --stdin.\n", 1},
		{[]string{"auth", "--provider", "vercel", "--stdin"}, "Unknown option or missing option value. Run foco --help.\n", 1},
		{[]string{"skill", "--agent", "Bad"}, "Usage: foco skill [--agent NAME] [--global] [--yes]\n", 1},
		{[]string{"doctor", "x"}, "This command takes no arguments.\n", 1},
		{[]string{"cache", "clear", "x"}, "Usage: foco \"question\" [root]. Run foco --help.\n", 1},
		{[]string{"q", "a", "b"}, "Usage: foco \"question\" [root]. Run foco --help.\n", 1},
		{[]string{"q", "--concurrency", "0"}, "--concurrency must be a positive integer.\n", 1},
		{[]string{"q", "--max-requests", "0"}, "--max-requests must be a positive integer.\n", 1},
		{[]string{"q", "--max-source-bytes=-1"}, "--max-source-bytes must be a nonnegative integer (0 means unlimited).\n", 1},
		{[]string{"q", "--max-output-bytes", "255"}, "--max-output-bytes must be 0 (unlimited) or at least 256.\n", 1},
		{[]string{"q", "--only", "nope"}, "Unknown or empty file category. Choose from: code, config, docs, data, audio, video, archives, logs, temp, unknown.\n", 1},
		{[]string{"q", "--exclude", "!keep.ts"}, "--exclude takes one gitignore pattern without a leading ! or # or a trailing unescaped \\.\n", 1},
		{[]string{"--accidentally-pasted-secret"}, "Unknown option or missing option value. Run foco --help.\n", 1},
	}
	for _, tc := range cases {
		var output, errorOutput bytes.Buffer
		code := Run(tc.args, &output, &errorOutput)
		if code != tc.code || output.String() != tc.want {
			t.Fatalf("%v: code=%d stdout=%q want=%q", tc.args, code, output.String(), tc.want)
		}
		if errorOutput.Len() != 0 {
			t.Fatalf("%v: stderr=%q", tc.args, errorOutput.String())
		}
		if strings.Contains(output.String(), "accidentally-pasted-secret") {
			t.Fatalf("%v: secret echoed", tc.args)
		}
	}
}

func TestParityHelpVersionAndStdoutStderrSeparation(t *testing.T) {
	var output, errorOutput bytes.Buffer
	if code := Run([]string{"--version"}, &output, &errorOutput); code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if output.String() != version.Version+"\n" {
		t.Fatalf("version stdout %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("version stderr %q", errorOutput.String())
	}

	output.Reset()
	errorOutput.Reset()
	if code := Run([]string{"--help"}, &output, &errorOutput); code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(output.String(), "Usage: foco \"question\" [root]") {
		t.Fatalf("help stdout %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("help stderr %q", errorOutput.String())
	}
}

func TestParityMissingCredentialsExitOneOnStdout(t *testing.T) {
	setConfigHome(t)
	var output, errorOutput bytes.Buffer
	code := Run([]string{"q", t.TempDir()}, &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Run foco auth or use foco auth --schema NAME --endpoint URL --model ID --stdin.") {
		t.Fatalf("stdout %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}
