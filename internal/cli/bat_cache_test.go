package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
)

func TestBatCacheInputDistinguishesAbsentAndPresentEmptyCaptures(t *testing.T) {
	home := t.TempDir()
	configKey := install.SourceCaptureKey{Target: filepath.Join(home, ".config", "bat", "config"), Source: batConfigSource}
	themeKey := install.SourceCaptureKey{Target: filepath.Join(home, ".config", "bat", "themes", "Carbonfox.tmTheme"), Source: batThemeSource}
	validConfig := install.CapturedSource{Content: []byte("--theme=\"Carbonfox\"\n"), ContentPresent: true}
	validTheme := install.CapturedSource{Content: []byte("<plist/>\n"), ContentPresent: true}
	tests := []struct {
		name     string
		captures map[install.SourceCaptureKey]install.CapturedSource
		want     string
	}{
		{name: "absent config", captures: map[install.SourceCaptureKey]install.CapturedSource{themeKey: validTheme}, want: "missing captured"},
		{name: "absent theme", captures: map[install.SourceCaptureKey]install.CapturedSource{configKey: validConfig}, want: "missing captured"},
		{name: "present empty config", captures: map[install.SourceCaptureKey]install.CapturedSource{configKey: {ContentPresent: true}, themeKey: validTheme}, want: "config is empty"},
		{name: "present empty theme", captures: map[install.SourceCaptureKey]install.CapturedSource{configKey: validConfig, themeKey: {ContentPresent: true}}, want: "theme is empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := batCacheInputFromCaptures(test.captures); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("batCacheInputFromCaptures() error = %v, want %q", err, test.want)
			}
		})
	}
	input, err := batCacheInputFromCaptures(map[install.SourceCaptureKey]install.CapturedSource{configKey: validConfig, themeKey: validTheme})
	if err != nil || string(input.Config) != string(validConfig.Content) || string(input.Theme) != string(validTheme.Content) {
		t.Fatalf("batCacheInputFromCaptures(valid) = %+v, %v", input, err)
	}
}
