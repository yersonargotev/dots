package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/selection"
)

const selectionChangeAcknowledgementRequiredCode = "selection-change-acknowledgement-required"

type selectionChangeErrorData struct {
	Code   string           `json:"code"`
	Change selection.Change `json:"selection_change"`
}

type selectionChangeAcknowledgementError struct {
	change selection.Change
}

func (e *selectionChangeAcknowledgementError) Error() string {
	return selectionChangeAcknowledgementRequiredCode +
		": removing previously selected Profiles or extra Tags requires --yes and --acknowledge-selection-change"
}

func (e *selectionChangeAcknowledgementError) JSONErrorData() any {
	return selectionChangeErrorData{
		Code:   selectionChangeAcknowledgementRequiredCode,
		Change: e.change,
	}
}

func guardSelectionChange(cmd *cobra.Command, effective *selection.Effective, dryRun, yes, acknowledge, alreadyAccepted bool) (bool, bool, error) {
	if effective.Report.Change == nil {
		return true, alreadyAccepted, nil
	}
	change := effective.Report.Change
	if alreadyAccepted {
		change.AcknowledgementAccepted = true
		return true, true, nil
	}

	if !change.AcknowledgementRequired {
		change.AcknowledgementAccepted = !dryRun
		if !wantsJSON(cmd) {
			renderSelectionChange(cmd.OutOrStdout(), *change)
		}
		return true, change.AcknowledgementAccepted, nil
	}

	if dryRun {
		if !wantsJSON(cmd) {
			renderSelectionChange(cmd.OutOrStdout(), *change)
		}
		return true, false, nil
	}

	if yes || wantsJSON(cmd) {
		change.AcknowledgementAccepted = yes && acknowledge
		if !wantsJSON(cmd) {
			renderSelectionChange(cmd.OutOrStdout(), *change)
		}
		if !change.AcknowledgementAccepted {
			return false, false, &selectionChangeAcknowledgementError{change: *change}
		}
		return true, true, nil
	}

	if !wantsJSON(cmd) {
		renderSelectionChange(cmd.OutOrStdout(), *change)
	}
	confirmed, err := confirmSelectionChange(cmd.InOrStdin(), cmd.OutOrStdout())
	if err != nil {
		return false, false, err
	}
	if !confirmed {
		fmt.Fprintln(cmd.OutOrStdout(), "Installed Selection change declined; operation canceled before mutation.")
		return false, false, nil
	}
	change.AcknowledgementAccepted = true
	fmt.Fprintln(cmd.OutOrStdout(), "Installed Selection change acknowledgement accepted.")
	return true, true, nil
}

func renderSelectionChange(w io.Writer, change selection.Change) {
	fmt.Fprintln(w, "Installed Selection change:")
	renderSelectionDelta(w, change.Delta)
	fmt.Fprintf(w, "  Acknowledgement: required=%t accepted=%t\n\n",
		change.AcknowledgementRequired, change.AcknowledgementAccepted)
}

func renderSelectionDelta(w io.Writer, delta selection.Delta) {
	fmt.Fprintf(w, "  Previous: profiles=%s extra-tags=%s effective-tags=%s\n",
		renderSelectionValues(delta.Previous.Profiles), renderSelectionValues(delta.Previous.ExtraTags), renderSelectionValues(delta.Previous.EffectiveTags))
	fmt.Fprintf(w, "  Requested: profiles=%s extra-tags=%s effective-tags=%s\n",
		renderSelectionValues(delta.Current.Profiles), renderSelectionValues(delta.Current.ExtraTags), renderSelectionValues(delta.Current.EffectiveTags))
	fmt.Fprintf(w, "  Added: profiles=%s extra-tags=%s effective-tags=%s managed-entries=%s dependencies=%s provisioners=%s\n",
		renderSelectionValues(delta.Added.Profiles), renderSelectionValues(delta.Added.ExtraTags),
		renderSelectionValues(delta.Added.EffectiveTags), renderSelectionValues(delta.Added.ManagedEntries),
		renderSelectionValues(delta.Added.Dependencies), renderSelectionValues(delta.Added.Provisioners))
	fmt.Fprintf(w, "  Removed: profiles=%s extra-tags=%s effective-tags=%s managed-entries=%s dependencies=%s provisioners=%s\n",
		renderSelectionValues(delta.Removed.Profiles), renderSelectionValues(delta.Removed.ExtraTags),
		renderSelectionValues(delta.Removed.EffectiveTags), renderSelectionValues(delta.Removed.ManagedEntries),
		renderSelectionValues(delta.Removed.Dependencies), renderSelectionValues(delta.Removed.Provisioners))
}

func confirmSelectionChange(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, "Apply this Installed Selection reduction? This is separate from Conflict Resolution. [y/N] ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read Installed Selection change confirmation: %w", err)
		}
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		return false, nil
	}
}
