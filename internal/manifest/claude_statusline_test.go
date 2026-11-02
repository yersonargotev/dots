package manifest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func runClaudeStatusline(t *testing.T, input string) string {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(repositoryRoot(t), "configs/claude/statusline-command.sh"))
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "LC_ALL=C", "TZ=UTC")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status line: %v\n%s", err, out)
	}
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(string(out), "")
}

func TestClaudeStatuslineModelEffortAndAutoMode(t *testing.T) {
	got := runClaudeStatusline(t, `{"model":{"display_name":"Opus"},"effort":{"level":"high"},"vim":{"mode":"NORMAL"},"permission_mode":"auto","context_window":{"total_input_tokens":12345,"used_percentage":12.34},"rate_limits":{"five_hour":{"used_percentage":23.4},"seven_day":{"used_percentage":41.2}}}`)
	want := "Opus (high) | N | auto | ctx: 12.3k 12.3% | 5h 77% left | 7d 59% left\n"
	if got != want {
		t.Fatalf("status line = %q, want %q", got, want)
	}
}

func TestClaudeStatuslineMissingZeroAndModeVariants(t *testing.T) {
	for _, tt := range []struct{ name, input, want string }{
		{"empty", `{}`, "\n"},
		{"nulls", `{"model":null,"effort":null,"vim":null,"permission_mode":null,"context_window":null,"rate_limits":null}`, "\n"},
		{"effort without model", `{"effort":{"level":"high"}}`, "\n"},
		{"camel field", `{"permissionMode":"auto"}`, "auto\n"},
		{"snake field wins", `{"permission_mode":"plan","permissionMode":"auto"}`, "plan\n"},
		{"null field fallback", `{"permission_mode":null,"permissionMode":"acceptEdits"}`, "accept edits\n"},
		{"bypass", `{"permission_mode":"bypassPermissions"}`, "bypass\n"},
		{"default", `{"permission_mode":"default"}`, "default\n"},
		{"unknown", `{"permission_mode":"future"}`, "\n"},
		{"vim alone", `{"vim":{"mode":"INSERT"}}`, "I\n"},
		{"zero values", `{"context_window":{"used_percentage":0,"total_input_tokens":0},"rate_limits":{"five_hour":{"used_percentage":0},"seven_day":{"used_percentage":100}}}`, "ctx: 0.0k 0.0% | 5h 100% left | 7d 0% left\n"},
		{"tokens only", `{"context_window":{"total_input_tokens":1500}}`, "ctx: 1.5k\n"},
		{"percentage only", `{"context_window":{"used_percentage":1.25}}`, "ctx: 1.2%\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := runClaudeStatusline(t, tt.input); got != tt.want {
				t.Fatalf("status line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClaudeStatuslineResetCountdowns(t *testing.T) {
	date, err := exec.LookPath("date")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "date"), "#!/bin/sh\nif [ \"$1\" = +%s ]; then echo 1700000000; else exec \""+date+"\" \"$@\"; fi\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tt := range []struct{ name, input, want string }{
		{"both countdowns", `{"rate_limits":{"five_hour":{"used_percentage":23.4,"resets_at":1700007500},"seven_day":{"used_percentage":41.2,"resets_at":1700183600}}}`, "2h:5m 77% left > 00:18 | 2d 3h 59% left > 17/11\n"},
		{"hours only", `{"rate_limits":{"seven_day":{"used_percentage":50,"resets_at":1700011100}}}`, "3h 50% left > 15/11\n"},
		{"elapsed and epoch zero", `{"rate_limits":{"five_hour":{"used_percentage":100,"resets_at":1699999999},"seven_day":{"used_percentage":0,"resets_at":0}}}`, "0h:0m 0% left > 22:13 | 0h 100% left > 01/01\n"},
		{"null resets", `{"rate_limits":{"five_hour":{"used_percentage":0,"resets_at":null},"seven_day":{"used_percentage":0}}}`, "5h 100% left | 7d 100% left\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := runClaudeStatusline(t, tt.input); got != tt.want {
				t.Fatalf("status line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClaudeStatuslineCatppuccinWithCarbonfoxSelected(t *testing.T) {
	root := repositoryRoot(t)
	for _, tt := range []struct {
		name, appearance, color string
		adaptive                bool
	}{
		{"fallback", "", "166;173;200", false},
		{"adaptive light", "", "92;95;119", true},
		{"adaptive dark", "Dark", "166;173;200", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".config/dots")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"theme.sh", "theme-carbonfox"} {
				data, err := os.ReadFile(filepath.Join(root, "configs/dots", name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.adaptive {
				if err := os.WriteFile(filepath.Join(dir, "adaptive-theme"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\necho Darwin\n")
			writeDefaultsStub(t, filepath.Join(bin, "defaults"), tt.appearance, 0)
			cmd := exec.Command("bash", filepath.Join(root, "configs/claude/statusline-command.sh"))
			cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			cmd.Stdin = strings.NewReader(`{"model":{"display_name":"Opus"}}`)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("status line: %v\n%s", err, out)
			}
			if want := "\x1b[38;2;" + tt.color + "mOpus\x1b[0m\n"; string(out) != want {
				t.Fatalf("palette output = %q, want %q", out, want)
			}
		})
	}
}
