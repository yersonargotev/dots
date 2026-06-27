package deps_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func TestInstallYesExecutesInstallableActionsThroughRunner(t *testing.T) {
	present := map[string]bool{"tmux": true, "brew": true, "sudo": true, "apt-get": true, "dnf": true, "pacman": true}
	look := func(command string) bool { return present[command] }
	runner := &recordingRunner{
		afterRun: func() {
			present["starship"] = true
		},
	}

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), deps.TierHomebrew, runner)
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
			present := map[string]bool{"tmux": true, "brew": true, "sudo": true, "apt-get": true, "dnf": true, "pacman": true}
			runner := &recordingRunner{afterRun: func() { present["starship"] = true }}

			if _, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
				return present[command]
			}, fontLookupSet(), tt.tier, runner); err != nil {
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
	present := map[string]bool{"tmux": true, "nvim": true, "brew": true, "sudo": true, "apt-get": true, "dnf": true, "pacman": true}
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
	}, fontLookupSet(), deps.TierHomebrew, runner); err != nil {
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

	report, err := deps.Install(manualOnlyManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), fontLookupSet(), deps.TierGeneric, runner)
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

func TestInstallYesDoesNotFailForUnresolvedOptionalDependencies(t *testing.T) {
	runner := &recordingRunner{}

	report, err := deps.Install(optionalManualManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), fontLookupSet(), deps.TierGeneric, runner)
	if err != nil {
		t.Fatalf("Install() error = %v, want optional manual dependency to be non-blocking", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls len = %d, want 0 for manual action (%#v)", len(runner.calls), runner.calls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusManual || report.Items[0].Requirement != manifest.DependencyRequirementOptional {
		t.Fatalf("report items = %#v, want one optional manual item", report.Items)
	}
}

func optionalManualManifest() manifest.Manifest {
	m := manualOnlyManifest()
	m.Entries[0].Dependencies[0].Requirement = manifest.DependencyRequirementOptional
	return m
}

func TestInstallYesContinuesAfterOptionalDependencyFailures(t *testing.T) {
	runnerErr := errors.New("optional package failed")
	present := map[string]bool{"brew": true}
	runner := &recordingRunner{err: runnerErr}

	report, err := deps.Install(optionalInstallableManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v, want optional install failure to be non-blocking", err)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusFailed || report.Items[0].Requirement != manifest.DependencyRequirementOptional {
		t.Fatalf("report items = %#v, want one optional failed item", report.Items)
	}
}

func optionalInstallableManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "starship", Requirement: manifest.DependencyRequirementOptional, Brew: "starship"}},
			},
		},
	}
}

func TestInstallYesReprobesAfterSuccessAndErrorsWhenStillMissing(t *testing.T) {
	var probes []string
	runner := &recordingRunner{}
	look := func(command string) bool {
		probes = append(probes, command)
		return command == "tmux" || command == "brew"
	}

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), deps.TierHomebrew, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved dependency error")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want 1", len(runner.calls))
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusUnresolved {
		t.Fatalf("report items = %#v, want one unresolved item", report.Items)
	}
	if !reflect.DeepEqual(probes, []string{"starship", "brew", "tmux", "starship"}) {
		t.Fatalf("probes = %#v, want initial plan probes then starship re-probe", probes)
	}
}

func TestInstallYesStopsOnFirstRunnerFailure(t *testing.T) {
	runnerErr := errors.New("package manager failed")
	runner := &recordingRunner{err: runnerErr}

	report, err := deps.Install(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), fontLookupSet(), deps.TierHomebrew, runner)
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

	report, err := deps.Install(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "starship"), fontLookupSet(), deps.TierHomebrew, runner)
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
	present := map[string]bool{"brew": true}
	runner := &recordingRunner{afterRun: func() { present["starship"] = true }}

	report, err := deps.Install(mixedManualAndInstallableManifest(), deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierHomebrew, runner)
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

func TestInstallYesExecutesHomebrewCaskActionsThroughRunner(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "CascadiaCode Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*"},
				},
			},
		},
	}
	fontPresent := false
	runner := &recordingRunner{afterRun: func() { fontPresent = true }}

	report, err := deps.Install(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), func(match string) bool {
		return fontPresent && match == "CascadiaCodeNF*"
	}, deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls len = %d, want 1 (%#v)", len(runner.calls), runner.calls)
	}
	if runner.calls[0].executable != "brew" {
		t.Fatalf("runner executable = %q, want brew", runner.calls[0].executable)
	}
	if !reflect.DeepEqual(runner.calls[0].args, []string{"install", "--cask", "font-cascadia-code-nf"}) {
		t.Fatalf("runner args = %#v, want brew install --cask font-cascadia-code-nf", runner.calls[0].args)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want one installed item", report.Items)
	}
}

func TestInstallYesAcceptsFallbackFontAfterCaskInstall(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*", FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"}},
				},
			},
		},
	}
	installedFallback := false
	runner := &recordingRunner{afterRun: func() { installedFallback = true }}

	report, err := deps.Install(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), func(match string) bool {
		return installedFallback && match == "CaskaydiaCoveNerdFont*"
	}, deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want installed when fallback font satisfies re-probe", report.Items)
	}
}

func TestInstallDryRunReportsInstallableActions(t *testing.T) {
	report, err := deps.InstallDryRun(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), fontLookupSet(), deps.TierHomebrew)
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
	report, err := deps.InstallDryRun(manualOnlyManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), fontLookupSet(), deps.TierGeneric)
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
	report, err := deps.InstallDryRun(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), fontLookupSet(), deps.TierHomebrew)
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

	report, err := deps.InstallDryRun(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("engram"), fontLookupSet(), deps.TierHomebrew)
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
	if item.TrustCommand != "brew trust --formula gentleman-programming/tap/gentle-ai" {
		t.Fatalf("Items[0].TrustCommand = %q, want formula trust guidance", item.TrustCommand)
	}
}

func TestInstallDryRunReportsMissingBootstrapManagerAsManual(t *testing.T) {
	report, err := deps.InstallDryRun(nodeToolchainManifest(), deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}
	item := report.Items[0]
	if item.Status != deps.InstallPreviewManual || item.Executable != "" || len(item.Bootstrap) != 1 || item.Manual == "" {
		t.Fatalf("Node dry-run item = %#v, want manual item that records non-runnable bootstrap", item)
	}
}

func TestInstallYesDoesNotRunBootstrapWhenManagerIsMissing(t *testing.T) {
	runner := &recordingRunner{}

	report, err := deps.Install(nodeToolchainManifest(), deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), deps.TierGeneric, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved required dependency")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want no fnm bootstrap when fnm is missing", runner.calls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusManual {
		t.Fatalf("report items = %#v, want one manual Node item", report.Items)
	}
}

func TestInstallDryRunReportsOfficialFNMInstallerOnLinuxWithoutProvider(t *testing.T) {
	report, err := deps.InstallDryRun(nodeToolchainManifest(), deps.Options{Profile: "default", OS: "linux"}, noProviderLookup("curl", "bash", "unzip"), fontLookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}
	item := report.Items[0]
	if item.Status != deps.InstallPreviewWouldInstall || item.Executable != "bash" || len(item.Args) != 2 || !strings.Contains(item.Args[1], "https://fnm.vercel.app/install") || !strings.Contains(item.Args[1], "--skip-shell") || len(item.Bootstrap) != 1 {
		t.Fatalf("Node dry-run item = %#v, want official fnm installer plus bootstrap", item)
	}
}

func TestInstallYesRunsOfficialFNMInstallerBeforeBootstrap(t *testing.T) {
	present := map[string]bool{"curl": true, "bash": true, "unzip": true}
	runner := &recordingRunner{}
	runner.afterRun = func() {
		last := runner.calls[len(runner.calls)-1]
		switch {
		case last.executable == "bash" && len(last.args) == 2 && strings.Contains(last.args[1], "https://fnm.vercel.app/install"):
			present["fnm"] = true
		case last.executable == "fnm" && reflect.DeepEqual(last.args, []string{"install", "--lts"}):
			present["node"] = true
		}
	}

	report, err := deps.Install(nodeToolchainManifest(), deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierGeneric, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls len = %d, want official installer then bootstrap (%#v)", len(runner.calls), runner.calls)
	}
	if runner.calls[0].executable != "bash" || !reflect.DeepEqual(runner.calls[0].args[:1], []string{"-c"}) || !strings.Contains(runner.calls[0].args[1], "curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell") {
		t.Fatalf("official fnm installer call = %#v", runner.calls[0])
	}
	if runner.calls[1].executable != "fnm" || !reflect.DeepEqual(runner.calls[1].args, []string{"install", "--lts"}) {
		t.Fatalf("bootstrap call = %#v", runner.calls[1])
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want installed Node item", report.Items)
	}
}

func TestInstallYesDoesNotRunFNMBootstrapWhenOfficialInstallerDoesNotExposeFNM(t *testing.T) {
	present := map[string]bool{"curl": true, "bash": true, "unzip": true}
	runner := &recordingRunner{}

	report, err := deps.Install(nodeToolchainManifest(), deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierGeneric, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved required dependency")
	}
	if len(runner.calls) != 1 || runner.calls[0].executable != "bash" {
		t.Fatalf("runner calls = %#v, want only official fnm installer", runner.calls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusUnresolved {
		t.Fatalf("report items = %#v, want unresolved Node item", report.Items)
	}
}

func nodeToolchainManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:          "Node LTS (fnm)",
				Commands:      []string{"fnm", "node"},
				Brew:          "fnm",
				LinuxHomebrew: true,
				Toolchain:     manifest.DependencyToolchainNodeLTSFNM,
			}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}
}

func TestInstallYesRunsConstrainedToolchainBootstrapBeforeReprobe(t *testing.T) {
	present := map[string]bool{"brew": true}
	runner := &recordingRunner{}
	runner.afterRun = func() {
		last := runner.calls[len(runner.calls)-1]
		switch {
		case last.executable == "brew" && reflect.DeepEqual(last.args, []string{"install", "fnm"}):
			present["fnm"] = true
		case last.executable == "fnm" && reflect.DeepEqual(last.args, []string{"install", "--lts"}):
			present["node"] = true
		}
	}

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:      "Node LTS (fnm)",
				Commands:  []string{"fnm", "node"},
				Brew:      "fnm",
				Toolchain: manifest.DependencyToolchainNodeLTSFNM,
			}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	report, err := deps.Install(m, deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierHomebrew, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	wantCalls := []runnerCall{
		{executable: "brew", args: []string{"install", "fnm"}},
		{executable: "fnm", args: []string{"install", "--lts"}},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled || !reflect.DeepEqual(report.Items[0].Bootstrap, []deps.Command{{Executable: "fnm", Args: []string{"install", "--lts"}}}) {
		t.Fatalf("report items = %#v, want installed item with fnm bootstrap", report.Items)
	}
}

func TestInstallDryRunReportsOfficialRustupInstaller(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:      "Rust stable (rustup)",
				Commands:  []string{"rustup", "rustc", "cargo"},
				Brew:      "rustup",
				Toolchain: manifest.DependencyToolchainRustStableRustup,
			}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	report, err := deps.InstallDryRun(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet("curl", "sh"), fontLookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}
	item := report.Items[0]
	if item.Status != deps.InstallPreviewWouldInstall || item.Executable != "sh" || len(item.Args) != 2 || !strings.Contains(item.Args[1], "https://sh.rustup.rs") || len(item.Bootstrap) != 1 {
		t.Fatalf("Rust dry-run item = %#v, want official installer plus bootstrap", item)
	}

	plan, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet("curl", "sh"), fontLookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Items) != 1 || !strings.Contains(plan.Items[0].Command, "sh -c '") || !strings.Contains(plan.Items[0].Command, `'\''=https'\''`) {
		t.Fatalf("Rust installer command hint = %#v, want shell-quoted script", plan.Items)
	}
}

func TestInstallYesRunsOfficialRustupInstallerBeforeBootstrap(t *testing.T) {
	present := map[string]bool{"curl": true, "sh": true}
	runner := &recordingRunner{}
	runner.afterRun = func() {
		last := runner.calls[len(runner.calls)-1]
		switch {
		case last.executable == "sh" && len(last.args) == 2 && strings.Contains(last.args[1], "https://sh.rustup.rs"):
			present["rustup"] = true
		case last.executable == "rustup" && reflect.DeepEqual(last.args, []string{"default", "stable"}):
			present["rustc"] = true
			present["cargo"] = true
		}
	}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:      "Rust stable (rustup)",
				Commands:  []string{"rustup", "rustc", "cargo"},
				Brew:      "rustup",
				Toolchain: manifest.DependencyToolchainRustStableRustup,
			}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	report, err := deps.Install(m, deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierGeneric, runner)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls len = %d, want official installer then bootstrap (%#v)", len(runner.calls), runner.calls)
	}
	if runner.calls[0].executable != "sh" || !reflect.DeepEqual(runner.calls[0].args[:1], []string{"-c"}) || !strings.Contains(runner.calls[0].args[1], "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path") {
		t.Fatalf("official installer call = %#v", runner.calls[0])
	}
	if runner.calls[1].executable != "rustup" || !reflect.DeepEqual(runner.calls[1].args, []string{"default", "stable"}) {
		t.Fatalf("bootstrap call = %#v", runner.calls[1])
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want installed Rust item", report.Items)
	}
}

func TestInstallYesExplainsRustupToolchainWhenProbesRemainMissing(t *testing.T) {
	present := map[string]bool{"rustup": true}
	runner := &recordingRunner{}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:      "Rust stable (rustup)",
				Commands:  []string{"rustup", "rustc", "cargo"},
				Brew:      "rustup",
				Toolchain: manifest.DependencyToolchainRustStableRustup,
			}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	report, err := deps.Install(m, deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierGeneric, runner)
	if err == nil {
		t.Fatalf("Install() error = nil, want unresolved rust toolchain error")
	}
	wantCalls := []runnerCall{{executable: "rustup", args: []string{"default", "stable"}}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if len(report.Items) != 1 || report.Items[0].Status != deps.InstallStatusUnresolved {
		t.Fatalf("report items = %#v, want one unresolved Rust item", report.Items)
	}
	manual := report.Items[0].Manual
	for _, want := range []string{"rustc", "cargo", "~/.cargo/bin", "rustup which rustc", "rustup which cargo"} {
		if !strings.Contains(manual, want) {
			t.Fatalf("Rust remediation missing %q: %q", want, manual)
		}
	}
}

func TestInstallYesRunsToolchainBootstrapWhenManagerAlreadyPresent(t *testing.T) {
	present := map[string]bool{"fnm": true}
	runner := &recordingRunner{afterRun: func() { present["node"] = true }}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{
			Tags:         []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "Node LTS (fnm)", Commands: []string{"fnm", "node"}, Brew: "fnm", Toolchain: manifest.DependencyToolchainNodeLTSFNM}},
		}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	if _, err := deps.Install(m, deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return present[command]
	}, fontLookupSet(), deps.TierGeneric, runner); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantCalls := []runnerCall{{executable: "fnm", args: []string{"install", "--lts"}}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
}
