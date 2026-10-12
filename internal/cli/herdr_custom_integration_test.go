package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomHerdrInstallRejectsInvalidSelectionBeforeMutation(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, goos, arch, want string
		tags                   []string
	}{
		{"both", "darwin", "arm64", "cannot be selected together", []string{"herdr", "herdr-custom"}},
		{"apple intel", "darwin", "amd64", "does not support", []string{"herdr-custom"}},
		{"linux arm", "linux", "arm64", "does not support", []string{"herdr-custom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, stateRoot, bin := t.TempDir(), t.TempDir(), t.TempDir()
			oldOS, oldArch := installHostOS, installHostArch
			installHostOS, installHostArch = tc.goos, tc.arch
			t.Cleanup(func() { installHostOS, installHostArch = oldOS, oldArch })
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PATH", bin)
			for _, name := range []string{"herdr", "herdr-custom", "brew", "apt-get", "sudo", "npx"} {
				writeHerdrIntegrationStub(t, filepath.Join(bin, name), "#!/bin/sh\nexit 97\n")
			}
			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := []string{"install", "--yes", "--skip-deps", "--file", filepath.Join(root, "dots.yaml"), "--source-root", root, "--home", home, "--state-root", stateRoot}
			for _, tag := range tc.tags {
				args = append(args, "--tag", tag)
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v output=%s", err, &out)
			}
			entries, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("mutated unsupported home: %v", entries)
			}
		})
	}
}
