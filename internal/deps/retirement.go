package deps

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

func retiredActionError(action InstallAction) error {
	d := manifest.Dependency{Name: action.Dependency, Command: action.Executable, Commands: append([]string{action.Probe}, action.Probes...)}
	if err := d.RetirementError(); err != nil {
		return err
	}
	for _, command := range action.Bootstrap {
		if strings.TrimSpace(command.Executable) == "herdr-custom" {
			return manifest.ErrHerdrCustomRetired
		}
	}
	if action.UserLocal != nil && (strings.TrimSpace(action.UserLocal.Recipe) == "herdr-custom" || strings.TrimSpace(action.UserLocal.Command) == "herdr-custom") {
		return manifest.ErrHerdrCustomRetired
	}
	return nil
}

func rejectRetiredPendingHome(home string, ops []journalOperation) (resultErr error) {
	for _, op := range ops {
		if op.Complete || retiredActionError(op.Captured.Action) == nil {
			continue
		}
		root, err := openReceiptRoot(home, false)
		if err != nil {
			return fmt.Errorf("inspect pending retired provider home: %w", err)
		}
		defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
		return rejectRetiredPending(root, ops)
	}
	return nil
}

// Denial depends on the captured implementation and physical home, never on
// whether the current manifest still declares or selects the retired provider.
// Completed receipts remain immutable audit evidence.
func rejectRetiredPending(home *receiptRoot, ops []journalOperation) error {
	for _, op := range ops {
		if op.Complete || retiredActionError(op.Captured.Action) == nil {
			continue
		}
		matches, err := homeScopeMatches(home, op.Captured)
		if err != nil {
			return fmt.Errorf("inspect pending custom provider journal %s; preserve pending evidence and external paths for manual remediation: %w", op.Captured.ID, err)
		}
		if matches {
			return fmt.Errorf("pending custom provider journal %s requires manual remediation; preserve journal, staging and external paths: %w", op.Captured.ID, manifest.ErrHerdrCustomRetired)
		}
	}
	return nil
}
