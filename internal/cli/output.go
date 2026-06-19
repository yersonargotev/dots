package cli

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// outputFlag is the persistent flag that selects the output surface.
const outputFlag = "output"

// outputText and outputJSON are the supported values of --output.
const (
	outputText = "text"
	outputJSON = "json"
)

// schemaVersion versions the agent output envelope independently of the dots
// binary. Renaming, removing, or repurposing a field in any command's JSON
// `data` is a breaking change to the Agent Output Contract and must bump this
// value.
const schemaVersion = "1"

// Envelope statuses. They mirror the semantic exit codes: ok->0, findings->2,
// error->1.
const (
	statusOK       = "ok"
	statusFindings = "findings"
	statusError    = "error"
)

// envelope is the single machine-readable shape every command emits under
// --output json. Data carries the command's domain report; Error is set only on
// the error envelope. The two are mutually exclusive.
type envelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	Data          any    `json:"data,omitempty"`
	Error         string `json:"error,omitempty"`
}

// findingReport is any command domain report that can report findings. Every
// read-only diagnostic report satisfies it, so renderOrEmit can keep the
// findings-and-output decision in one place.
type findingReport interface {
	HasFindings() bool
}

// wantsJSON reports whether the caller selected Machine Output Mode for this
// command. It reads the inherited persistent --output flag.
func wantsJSON(cmd *cobra.Command) bool {
	v, err := cmd.Flags().GetString(outputFlag)
	return err == nil && v == outputJSON
}

// commandName is the command path without the root binary name, e.g. "status"
// or "deps check". It labels the envelope so an agent can route on it.
func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}

// renderOrEmit is the single seam every read-only diagnostic command routes its
// output through. In Machine Output Mode it emits the JSON envelope; otherwise
// it runs the text renderer. Either way, a report with findings yields a
// FindingsError so the exit code and the envelope status always agree.
func renderOrEmit(cmd *cobra.Command, report findingReport, renderText func() error) error {
	if wantsJSON(cmd) {
		return emitReport(cmd, report)
	}
	if err := renderText(); err != nil {
		return err
	}
	if report.HasFindings() {
		return &FindingsError{}
	}
	return nil
}

// emitReport writes a command's domain report as a JSON envelope to stdout and,
// when the report carries findings, returns a FindingsError so Run maps it to
// the findings exit code.
func emitReport(cmd *cobra.Command, report findingReport) error {
	findings := report.HasFindings()
	status := statusOK
	if findings {
		status = statusFindings
	}
	if err := writeEnvelope(cmd.OutOrStdout(), envelope{
		SchemaVersion: schemaVersion,
		Command:       commandName(cmd),
		Status:        status,
		Data:          report,
	}); err != nil {
		return err
	}
	if findings {
		return &FindingsError{}
	}
	return nil
}

// emitError writes the error envelope for Machine Output Mode to stdout, so an
// agent always receives parseable output instead of a human "Error:" line.
func emitError(w io.Writer, cmd *cobra.Command, cause error) {
	_ = writeEnvelope(w, envelope{
		SchemaVersion: schemaVersion,
		Command:       commandName(cmd),
		Status:        statusError,
		Error:         cause.Error(),
	})
}

func writeEnvelope(w io.Writer, env envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
