package manifest

import (
	"errors"
	"slices"
	"strings"
)

// ErrHerdrCustomRetired denies execution, without invalidating historical
// Installation Metadata or granting authority to remove the existing environment.
var ErrHerdrCustomRetired = errors.New("herdr-custom is retired; submit the complete retained selection with --profile, --tag and --exclude-tag, dropping herdr-custom and excluding herdr from Profiles when preserving the existing environment; reductions require --yes --acknowledge-selection-change (or confirmed interactive acknowledgement); use --clear-selection with its safeguards for empty intent; nothing is replaced or uninstalled; see docs/herdr.md")

func rejectRetiredTags(tags []string) error {
	if slices.Contains(tags, "herdr-custom") {
		return ErrHerdrCustomRetired
	}
	return nil
}

// RetirementError checks executable dependency identity independently of the
// selecting Tag. Historical readers deliberately do not apply this execution gate.
func (d Dependency) RetirementError() error {
	if strings.TrimSpace(d.Name) == "herdr-custom" || strings.TrimSpace(d.Command) == "herdr-custom" || slices.Contains(d.Probes(), "herdr-custom") ||
		d.UserLocal != nil && strings.TrimSpace(d.UserLocal.Recipe) == "herdr-custom" ||
		d.RollingUserLocal != nil && strings.TrimSpace(d.RollingUserLocal.Recipe) == "herdr-custom" {
		return ErrHerdrCustomRetired
	}
	return nil
}

func (m Manifest) validateRetiredExecution() error {
	if _, retired := m.Tags["herdr-custom"]; retired {
		return ErrHerdrCustomRetired
	}
	for _, tag := range m.Tags {
		if err := rejectRetiredTags(tag.ReplacedBy); err != nil {
			return err
		}
	}
	for _, profile := range m.Profiles {
		if err := rejectRetiredTags(profile.Tags); err != nil {
			return err
		}
	}
	for _, set := range m.Dependencies {
		if err := rejectRetiredTags(set.Tags); err != nil {
			return err
		}
		for _, d := range set.Dependencies {
			if err := d.RetirementError(); err != nil {
				return err
			}
		}
	}
	for _, e := range m.Entries {
		if err := rejectRetiredTags(e.Tags); err != nil {
			return err
		}
		if _, retired := e.SourceOverrides["herdr-custom"]; retired {
			return ErrHerdrCustomRetired
		}
		for _, d := range e.Dependencies {
			if err := d.RetirementError(); err != nil {
				return err
			}
		}
	}
	for _, p := range m.Provisioners {
		if err := rejectRetiredTags(p.Tags); err != nil {
			return err
		}
		if err := rejectRetiredTags(p.RequiredTags); err != nil {
			return err
		}
		if p.Tool == "herdr-custom" {
			return ErrHerdrCustomRetired
		}
		for _, d := range p.Dependencies {
			if err := d.RetirementError(); err != nil {
				return err
			}
		}
	}
	return nil
}
