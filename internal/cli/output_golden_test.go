package cli

import (
	"bytes"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/deps/pkgmgr"
	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/gitrepo"
	inst "github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionmigration"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	"github.com/yersonargotev/dots/internal/upgrade"
)

// TestEnvelopeGolden locks the full JSON shape of the Agent Output Contract.
// Any rename, removal, or addition of an exposed domain field changes a golden
// file and must be a deliberate schema_version bump. The fixtures also prove the
// machine-local / advisory fields (resolved_source, probe_detail, hint,
// package-manager paths/guidance) stay out of the contract.
func TestEnvelopeGolden(t *testing.T) {
	installPreview := deps.InstallDryRunReport{Profile: "default", Tier: deps.TierHomebrew, Items: []deps.InstallPreview{
		{Dependency: "starship", Requirement: "required", Status: deps.InstallPreviewWouldInstall, Provider: deps.TierHomebrew, Package: "starship", Executable: "brew", Args: []string{"install", "starship"}},
	}}
	installResult := deps.InstallReport{Profile: "default", Tier: deps.TierHomebrew, Items: []deps.InstallItem{
		{Dependency: "starship", Requirement: "required", Status: deps.InstallStatusInstalled, Provider: deps.TierHomebrew, Package: "starship", Executable: "brew", Args: []string{"install", "starship"}},
	}}
	selectionDelta := selection.Delta{
		Previous: selection.Snapshot{Profiles: []string{"core"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"}},
		Current:  selection.Snapshot{Profiles: []string{"core"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "desktop", "work"}},
		Added: selection.Changes{
			EffectiveTags: []string{"desktop"}, ManagedEntries: []string{"~/.config/ghostty/config"}, Dependencies: []string{}, Provisioners: []string{},
		},
		Removed: selection.Changes{
			EffectiveTags: []string{}, ManagedEntries: []string{}, Dependencies: []string{}, Provisioners: []string{},
		},
		MissingProfiles: []string{},
		StaleExtraTags:  []string{},
	}
	evolvedSelection := selection.Report{
		Source: selection.SourceRecorded, Profiles: []string{"core"}, ExtraTags: []string{"work"},
		EffectiveTags: []string{"core", "desktop", "work"}, Delta: &selectionDelta,
	}
	blockingDelta := selection.Delta{
		Previous: selection.Snapshot{Profiles: []string{"core"}, ExtraTags: []string{"retired"}, EffectiveTags: []string{"core", "retired"}},
		Current:  selection.Snapshot{Profiles: []string{"core"}, ExtraTags: []string{"retired"}, EffectiveTags: []string{"core", "retired"}},
		Added: selection.Changes{
			EffectiveTags: []string{}, ManagedEntries: []string{}, Dependencies: []string{}, Provisioners: []string{},
		},
		Removed: selection.Changes{
			EffectiveTags: []string{}, ManagedEntries: []string{"~/.retired"}, Dependencies: []string{}, Provisioners: []string{},
		},
		MissingProfiles: []string{},
		StaleExtraTags:  []string{"retired"},
	}
	tests := []struct {
		name   string
		env    envelope
		golden string
	}{
		{
			name: "status",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "status",
				Status:        statusFindings,
				Data: statusReport{
					Profile: "default",
					Selection: &selection.Report{
						Source: selection.SourceExplicit, Profiles: []string{"default"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"},
					},
					Entries: []status.Entry{
						{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", State: status.StateOK},
						{Source: "configs/git/config", Target: "/home/user/.gitconfig", Strategy: "copy", State: status.StateMissing},
						{Source: "configs/zellij/default.kdl", Target: "/home/user/.config/zellij/config.kdl", Strategy: "symlink", State: status.StateConflict, Reason: plan.ConflictReasonSourceOverrideNotSelected, MatchingTags: []string{"adaptive-theme"}},
					},
					Provisioners: provision.StatusReport{
						Profile: "default",
						Summary: provision.StatusSummary{State: provision.SummaryStatePending, Pending: 1},
						Items: []provision.StatusItem{
							{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global", "--agents", "claude-code"}, Targets: []string{"~/.claude", "~/.gentle-ai"}, Status: provision.StatusStatePending},
						},
					},
				},
			},
			golden: "envelope_status.golden",
		},
		{
			name: "installed",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "installed",
				Status:        statusOK,
				Data: inst.Report{
					Metadata:   inst.MetadataSummary{Path: "/home/user/.local/state/dots/installed.json", Version: 3},
					Provenance: state.Provenance{SourceRoot: "/src/dots", SourceRevision: "abc123", DotsVersion: "v0.test", RecordedAt: "2026-07-08T12:00:00Z"},
					InstalledSelection: &state.InstalledSelection{
						Profiles:     []string{"core", "agents"},
						ExtraTags:    []string{"work"},
						ResolvedTags: []string{"core", "agents", "work"},
						Provenance:   state.Provenance{SourceRoot: "/src/dots", SourceRevision: "abc123", DotsVersion: "v0.test", RecordedAt: "2026-07-08T12:00:00Z"},
					},
					ManagedEntries: []inst.ManagedEntry{{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", InstalledAt: "2026-07-08T12:00:00Z", Tags: []string{"core"}, TagsSource: "recorded", Profiles: []string{"core"}, ProfilesSource: "recorded", ManifestMatched: true}},
					Tags:           []string{"core"},
					Profiles:       []inst.ProfileCoverage{{Name: "core", Source: "recorded+inferred", State: inst.CoverageComplete, CoveredTags: []string{"core"}, CoveredEntries: 1, TotalEntries: 1}},
					Provisioners:   []inst.ProvisionerRun{{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Status: "provisioned", LastRunAt: "2026-07-08T12:00:00Z", Profile: "core", Profiles: []string{"core"}, ProfilesSource: "recorded", Tags: []string{"core"}, TagsSource: "recorded", ManifestMatched: true}},
				},
			},
			golden: "envelope_installed.golden",
		},
		{
			name: "selection migration required",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "status",
				Status:        statusError,
				Data: selectionMigrationErrorData{
					Code: selectionMigrationRequiredCode,
					Candidate: &selectionmigration.Candidate{
						Profiles:           []string{"core"},
						ExtraTags:          []string{"adaptive-theme"},
						EffectiveTags:      []string{"core", "adaptive-theme"},
						Confidence:         selectionmigration.ConfidenceHigh,
						AmbiguityReasons:   []selectionmigration.AmbiguityReason{},
						RecommendedCommand: "dots install --profile core --tag adaptive-theme",
					},
					Remediation: selectionMigrationRemediation{
						RecommendedCommand: "dots install --profile core --tag adaptive-theme",
					},
				},
				Error: "selection-migration-required: Installation Metadata predates authoritative Installed Selection; run dots install --profile core --tag adaptive-theme",
			},
			golden: "envelope_selection_migration_required.golden",
		},
		{
			name: "plan",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "plan",
				Status:        statusFindings,
				Data: plan.Plan{Profile: "default", Selection: &selection.Report{
					Source: selection.SourceExplicit, Profiles: []string{"default"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"},
				}, Actions: []plan.Action{
					{Source: "configs/zsh/zshrc", ResolvedSource: "/abs/machine/local/path/MUST/NOT/APPEAR", Target: "/home/user/.zshrc", Strategy: "symlink", Status: plan.StatusCreate},
					{Source: "configs/git/config", ResolvedSource: "/abs/x", Target: "/home/user/.gitconfig", Strategy: "copy", Status: plan.StatusConflict, Reason: plan.ConflictReasonSourceOverrideNotSelected, MatchingTags: []string{"adaptive-theme"}},
				}},
			},
			golden: "envelope_plan.golden",
		},
		{
			name: "doctor",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "doctor",
				Status:        statusFindings,
				Data: doctor.Report{
					Profile: "default",
					Selection: &selection.Report{
						Source: selection.SourceExplicit, Profiles: []string{"default"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"},
					},
					OS:       "darwin",
					Platform: doctor.Platform{Supported: true, OS: "darwin"},
					Dependencies: deps.CheckReport{Profile: "default", Results: []deps.Result{
						{Name: "git", Requirement: "required", Command: "git", Present: true, Warning: "git on PATH but `git --version` failed", ProbeDetail: "MUST NOT APPEAR", Hint: "MUST NOT APPEAR"},
						{Name: "rg", Requirement: "optional", Command: "rg", Present: false},
					}},
					Configuration: status.Report{Profile: "default", Entries: []status.Entry{
						{Source: "configs/zsh/zshrc", Target: "/Users/me/.zshrc", Strategy: "symlink", State: status.StateOK},
					}},
					Provisioners: provision.CheckReport{Profile: "default", Items: []provision.Readiness{
						{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Missing: []string{"engram"}},
					}},
					SecretScan: doctor.SecretReport{Findings: []doctor.SecretFinding{
						{Source: "configs/git/config", Line: 3, Pattern: "credential-assignment"},
					}},
				},
			},
			golden: "envelope_doctor.golden",
		},
		{
			name: "deps_check",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "deps check",
				Status:        statusFindings,
				Data: deps.CheckReport{Profile: "default", Selection: &selection.Report{
					Source: selection.SourceExplicit, Profiles: []string{"default"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"},
				}, Results: []deps.Result{
					{Name: "git", Requirement: "required", Command: "git", Present: true, ProbeDetail: "MUST NOT APPEAR", Hint: "MUST NOT APPEAR"},
					{Name: "rg", Requirement: "optional", Command: "rg", Present: false},
				}},
			},
			golden: "envelope_deps_check.golden",
		},
		{
			name: "deps_plan",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "deps plan",
				Status:        statusFindings,
				Data: deps.PlanReport{
					Profile: "default",
					Selection: &selection.Report{
						Source: selection.SourceExplicit, Profiles: []string{"default"}, ExtraTags: []string{"work"}, EffectiveTags: []string{"core", "work"},
					},
					Tier: deps.Tier("debian"),
					Actions: []deps.InstallAction{
						{Dependency: "rg", Requirement: "optional", Status: deps.InstallActionStatusInstallable, Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}, Candidates: []deps.ProviderCandidate{{Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "ripgrep"}}}},
					},
					Items: []deps.Guidance{
						{Name: "rg", Requirement: "optional", Command: "apt-get install -y ripgrep", Action: deps.InstallAction{Dependency: "rg", Requirement: "optional", Status: deps.InstallActionStatusInstallable, Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}, Candidates: []deps.ProviderCandidate{{Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "ripgrep"}}}}},
					},
				},
			},
			golden: "envelope_deps_plan.golden",
		},
		{
			name: "install",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "install",
				Status:        statusOK,
				Data: installReport{
					DryRun: false,
					PackageManagerSetup: &pkgmgr.Report{
						Manager: "homebrew",
						Status:  pkgmgr.StatusInstalled,
						Command: pkgmgr.HomebrewInstallerCommand(),
						Detection: pkgmgr.HomebrewDetection{
							Found:        true,
							Path:         "/opt/homebrew/bin/brew-MUST-NOT-APPEAR",
							NeedsPATH:    true,
							PATHGuidance: "MUST NOT APPEAR",
						},
						Reason:       "selected required Dependencies need Homebrew, but brew is not available",
						Dependencies: []string{"starship"},
					},
					Dependencies: &installDependenciesReport{
						Preview: &installPreview,
						Result:  &installResult,
					},
					Plan: plan.Plan{Profile: "default", Actions: []plan.Action{
						{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", Status: plan.StatusCreate},
					}},
					Provisioners: provision.Plan{Profile: "default", Steps: []provision.Step{
						{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global", "--agents", "claude-code"}, Targets: []string{"~/.claude", "~/.gentle-ai"}, GlobalTools: []string{"claude (~/.local/bin via npm prefix)"}},
					}},
					BackupSets: []installBackupSetReport{{
						BackupSet: backups.BackupSet{ID: "backup-20260627T140000.000000000Z", CreatedAt: "2026-06-27T14:00:00Z", Reason: "pre-install conflict protection", Targets: []string{"/home/user/.gitconfig"}},
						Path:      "/home/user/.local/state/dots/backups/backup-20260627T140000.000000000Z",
					}},
				},
			},
			golden: "envelope_install.golden",
		},
		{
			name: "update",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "update",
				Status:        statusOK,
				Data: updateReport{
					DryRun:    false,
					Selection: evolvedSelection,
					Update:    gitrepo.Update{OldRev: "abc123", NewRev: "def456", Incoming: []string{"def456 update managed config"}},
					Plan:      plan.Plan{Profile: "core", Profiles: []string{"core"}, Tags: []string{"core", "desktop", "work"}, Selection: &evolvedSelection, Actions: []plan.Action{}},
					Provisioners: provision.Plan{
						Profile: "core", Profiles: []string{"core"}, Tags: []string{"core", "desktop", "work"}, Steps: []provision.Step{},
					},
				},
			},
			golden: "envelope_update.golden",
		},
		{
			name: "update blocking selection evolution",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "update",
				Status:        statusError,
				Data: struct {
					SelectionDelta selection.Delta `json:"selection_delta"`
				}{SelectionDelta: blockingDelta},
				Error: `recorded selection: extra Tag "retired" is no longer declared; update the selection before refreshing`,
			},
			golden: "envelope_update_selection_error.golden",
		},
		{
			name: "upgrade",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "upgrade",
				Status:        statusOK,
				Data: upgradeReport{
					DryRun:    false,
					Selection: evolvedSelection,
					Binary:    upgrade.Plan{Channel: upgrade.ChannelHomebrew, CurrentVersion: "v0.63.0", LatestVersion: "v0.64.0", Action: upgrade.ActionHomebrewUpgrade, Executable: "/opt/homebrew/bin/dots"},
					Update:    gitrepo.Update{OldRev: "abc123", NewRev: "def456", Incoming: []string{"def456 update managed config"}},
					Plan:      plan.Plan{Profile: "core", Profiles: []string{"core"}, Tags: []string{"core", "desktop", "work"}, Selection: &evolvedSelection, Actions: []plan.Action{}},
					Provisioners: provision.Plan{
						Profile: "core", Profiles: []string{"core"}, Tags: []string{"core", "desktop", "work"}, Steps: []provision.Step{},
					},
				},
			},
			golden: "envelope_upgrade.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeEnvelope(&out, tt.env); err != nil {
				t.Fatalf("writeEnvelope: %v", err)
			}
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}
