package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/batcache"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
)

const (
	batConfigSource = "configs/bat/config-carbonfox"
	batThemeSource  = "configs/themes/carbonfox.tmTheme"
)

func hasBatCacheStep(p provision.Plan) bool {
	for _, step := range p.Steps {
		if step.Tool == "bat" && step.Executable == "bat" && len(step.Args) == 2 && step.Args[0] == "cache" && step.Args[1] == "--build" {
			return true
		}
	}
	return false
}

func captureBatCacheInput(p plan.Plan, opts install.Options) (map[install.SourceCaptureKey]install.CapturedSource, *batcache.Input, error) {
	selected := p
	selected.Actions = nil
	for _, action := range p.Actions {
		isConfig := action.Source == batConfigSource && filepath.Clean(action.Target) == filepath.Join(opts.Home, ".config", "bat", "config")
		isTheme := action.Source == batThemeSource && filepath.Clean(action.Target) == filepath.Join(opts.Home, ".config", "bat", "themes", "Carbonfox.tmTheme")
		if isConfig || isTheme {
			selected.Actions = append(selected.Actions, action)
		}
	}
	if len(selected.Actions) != 2 {
		return nil, nil, fmt.Errorf("bat cache activation requires exactly two managed inputs, found %d", len(selected.Actions))
	}
	captures, err := install.CaptureManagedSources(selected, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("capture bat cache inputs: %w", err)
	}
	input, err := batCacheInputFromCaptures(captures)
	if err != nil {
		return nil, nil, errors.Join(err, install.ReleaseCapturedSources(captures))
	}
	return captures, input, nil
}

func batCacheInputFromCaptures(captures map[install.SourceCaptureKey]install.CapturedSource) (*batcache.Input, error) {
	var config, theme []byte
	var configPresent, themePresent bool
	for key, capture := range captures {
		if !capture.ContentPresent {
			continue
		}
		switch key.Source {
		case batConfigSource:
			configPresent = true
			config = append([]byte(nil), capture.Content...)
		case batThemeSource:
			if filepath.Base(key.Target) == "Carbonfox.tmTheme" {
				themePresent = true
				theme = append([]byte(nil), capture.Content...)
			}
		}
	}
	if !configPresent || !themePresent {
		return nil, fmt.Errorf("bat cache activation is missing captured Carbonfox config or theme bytes")
	}
	if len(config) == 0 {
		return nil, fmt.Errorf("bat cache activation captured Carbonfox config is empty")
	}
	if len(theme) == 0 {
		return nil, fmt.Errorf("bat cache activation captured Carbonfox theme is empty")
	}
	return &batcache.Input{Config: config, Theme: theme}, nil
}
