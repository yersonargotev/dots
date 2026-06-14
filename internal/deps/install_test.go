package deps_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func TestInstallYesExecutesInstallableActionsThroughRunner(t *testing.T) {
	present := map[string]bool{"tmux": true}
	look := func(command string) bool { return present[command] }
	runner := &recordingRunner{
		afterRun: func() {
			present["starship"] = true
		},
	}

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want 1 (%#v)", len(runner.calls), runner.calls)
	}
	if runner.calls[0].executable != "brew" {
		t.Fatalf("runner executable = %q, want brew", runner.calls[0].executable)
	}
	if !reflect.DeepEqual(runner.calls[0].args, []string{"install", "starship"}) {
		t.Fatalf("runner args = %#v, want brew install starship", runner.calls[0].args)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want one installed item", report.Items)
	}
}

func TestInstallYesUsesPackageManagerConfirmationArgs(t *testing.T) {
	tests := []struct {
		name       string
		tier       deps.Tier
		executable string
		args       []string
	}{
		{name: "homebrew", tier: deps.TierHomebrew, executable: "brew", args: []string{"install", "starship"}},
		{name: "debian", tier: deps.TierDebian, executable: "sudo", args: []string{"apt-get", "install", "-y", "starship"}},
		{name: "fedora", tier: deps.TierFedora, executable: "sudo", args: []string{"dnf", "install", "-y", "starship"}},
		{name: "arch", tier: deps.TierArch, executable: "sudo", args: []string{"pacman", "-S", "--noconfirm", "starship"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present := map[string]bool{"tmux": true}
			runner := &recordingRunner{afterRun: func() { present["starship"] = true }}

			if _, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
				return present[command]
			}, tt.tier, runner); err != nil {
				t.Fatalf("Install() error = %v", err)
			}

			if len(runner.calls) != 1 {
				t.Fatalf("runner calls len = %d, want 1", len(runner.calls))
			}
			if runner.calls[0].executable != tt.executable {
				t.Fatalf("runner executable = %q, want %q", runner.calls[0].executable, tt.executable)
			}
			if !reflect.DeepEqual(runner.calls[0].args, tt.args) {
				t.Fatalf("runner args = %#v, want %#v", runner.calls[0].args, tt.args)
			}
		})
	}
}

func TestInstallYesExecutesMissingInstallableActionsInPlanOrder(t *testing.T) {
	present := map[string]bool{"tmux": true, "nvim": true}
	runner := &recordingRunner{}
	runner.afterRun = func() {
		last := runnerLastPackage(t, runner)
		if last == "ripgrep" {
			present["rg"] = true
			return
		}
		present[last] = true
	}

	if _, err := deps.Install(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		return present[command]
	}, deps.TierHomebrew, runner); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	got := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		got = append(got, call.args[len(call.args)-1])
	}
	want := []string{"starship", "ripgrep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runner package order = %#v, want %#v", got, want)
	}
}

func TestInstallYesDoesNotRunManualActionsAndReportsUnresolved(t *testing.T) {
	runner := &recordingRunner{}

	report, err := deps.Install(manualOnlyManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), deps.TierGeneric, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved manual dependency error")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls len = %d, want 0 for manual action (%#v)", len(runner.calls), runner.calls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusManual {
		t.Fatalf("report items = %#v, want one manual item", report.Items)
	}
}

func TestInstallYesReprobesAfterSuccessAndErrorsWhenStillMissing(t *testing.T) {
	var probes []string
	runner := &recordingRunner{}
	look := func(command string) bool {
		probes = append(probes, command)
		return command == "tmux"
	}

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, deps.TierHomebrew, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved dependency error")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want 1", len(runner.calls))
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusUnresolved {
		t.Fatalf("report items = %#v, want one unresolved item", report.Items)
	}
	if !reflect.DeepEqual(probes, []string{"starship", "tmux", "starship"}) {
		t.Fatalf("probes = %#v, want initial plan probes then starship re-probe", probes)
	}
}

func TestInstallYesStopsOnFirstRunnerFailure(t *testing.T) {
	runnerErr := errors.New("package manager failed")
	runner := &recordingRunner{err: runnerErr}

	report, err := deps.Install(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), deps.TierHomebrew, runner)
	if !errors.Is(err, runnerErr) {
		t.Fatalf("Install() error = %v, want runner error", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want stop after first failure (%#v)", len(runner.calls), runner.calls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusFailed {
		t.Fatalf("report items = %#v, want one failed item", report.Items)
	}
}

func TestInstallYesSkipsAlreadyPresentDependenciesBeforeExecution(t *testing.T) {
	runner := &recordingRunner{}

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "starship"), deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls len = %d, want 0 for already-present dependencies", len(runner.calls))
	}
	if len(report.Items) != 0 {
		t.Fatalf("report items len = %d, want 0 when no install actions are needed", len(report.Items))
	}
}

func TestInstallYesAllowsProgressWithManualAndInstallableDependencies(t *testing.T) {
	present := map[string]bool{}
	runner := &recordingRunner{afterRun: func() { present["starship"] = true }}

	report, err := deps.Install(mixedManualAndInstallableManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		return present[command]
	}, deps.TierHomebrew, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved manual dependency error")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want installable action to run despite manual dependency", len(runner.calls))
	}
	gotStatuses := []deps.InstallStatus{}
	for _, item := range report.Items {
		gotStatuses = append(gotStatuses, item.Status)
	}
	wantStatuses := []deps.InstallStatus{deps.InstallStatusManual, deps.InstallStatusInstalled}
	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Fatalf("report statuses = %#v, want %#v", gotStatuses, wantStatuses)
	}
}

func mixedManualAndInstallableManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "neovim", Command: "nvim"},
					{Name: "starship", Brew: "starship"},
				},
			},
		},
	}
}

func runnerLastPackage(t *testing.T, runner *recordingRunner) string {
	t.Helper()
	if len(runner.calls) == 0 {
		t.Fatalf("runner has no calls")
	}
	args := runner.calls[len(runner.calls)-1].args
	if len(args) == 0 {
		t.Fatalf("runner last call has no args")
	}
	return args[len(args)-1]
}

type runnerCall struct {
	executable string
	args       []string
}

type recordingRunner struct {
	calls    []runnerCall
	afterRun func()
	err      error
}

func (r *recordingRunner) Run(executable string, args []string) error {
	r.calls = append(r.calls, runnerCall{executable: executable, args: append([]string(nil), args...)})
	if r.err != nil {
		return r.err
	}
	if r.afterRun != nil {
		r.afterRun()
	}
	return nil
}

func TestInstallDryRunReportsInstallableActions(t *testing.T) {
	report, err := deps.InstallDryRun(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}

	if report.Profile != "default" {
		t.Fatalf("Profile = %q, want default", report.Profile)
	}
	if report.Tier != deps.TierHomebrew {
		t.Fatalf("Tier = %q, want homebrew", report.Tier)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}

	item := report.Items[0]
	if item.Dependency != "starship" {
		t.Fatalf("Items[0].Dependency = %q, want starship", item.Dependency)
	}
	if item.Status != deps.InstallPreviewWouldInstall {
		t.Fatalf("Items[0].Status = %q, want %q", item.Status, deps.InstallPreviewWouldInstall)
	}
	if item.Package != "starship" || item.Executable != "brew" {
		t.Fatalf("Items[0] package/executable = %#v, want brew install starship", item)
	}
	if !reflect.DeepEqual(item.Args, []string{"install", "starship"}) {
		t.Fatalf("Items[0].Args = %#v, want install starship", item.Args)
	}
	if item.Manual != "" {
		t.Fatalf("Items[0].Manual = %q, want empty", item.Manual)
	}
}

func TestInstallDryRunReportsManualActionsAsManual(t *testing.T) {
	report, err := deps.InstallDryRun(manualOnlyManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}

	item := report.Items[0]
	if item.Status != deps.InstallPreviewManual {
		t.Fatalf("Status = %q, want %q", item.Status, deps.InstallPreviewManual)
	}
	if item.Executable != "" || len(item.Args) != 0 || item.Package != "" {
		t.Fatalf("manual item has executable fields: %#v", item)
	}
	if item.Manual == "" {
		t.Fatalf("Manual empty, want manual guidance")
	}
}

func manualOnlyManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "neovim", Command: "nvim"}},
			},
		},
	}
}

func TestInstallDryRunUsesPlanSelectionOrderAndSkipsPresentDependencies(t *testing.T) {
	report, err := deps.InstallDryRun(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}

	got := make([]string, 0, len(report.Items))
	for _, item := range report.Items {
		got = append(got, item.Dependency)
	}
	want := []string{"starship", "ripgrep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dry-run item order = %#v, want %#v", got, want)
	}
}

func installSelectionManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "tmux", Brew: "tmux"},
					{Name: "starship", Brew: "starship"},
					{Name: "ripgrep", Command: "rg", Brew: "ripgrep"},
				},
			},
			{
				Source: "configs/tmux/tmux.conf", Target: "~/.tmux.conf", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "starship", Brew: "starship"},
					{Name: "neovim", Command: "nvim", Brew: "neovim"},
				},
			},
		},
	}
}

func TestInstallDryRunIncludesSelectedProvisionerDependencies(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "gentle-ai",
				Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "gentle-ai", Brew: "gentleman-programming/tap/gentle-ai"},
					{Name: "engram", Brew: "gentleman-programming/tap/engram"},
				},
			},
		},
	}

	report, err := deps.InstallDryRun(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("engram"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}
	item := report.Items[0]
	if item.Dependency != "gentle-ai" {
		t.Fatalf("Items[0].Dependency = %q, want gentle-ai", item.Dependency)
	}
	if item.Package != "gentleman-programming/tap/gentle-ai" {
		t.Fatalf("Items[0].Package = %q, want tap package", item.Package)
	}
}
