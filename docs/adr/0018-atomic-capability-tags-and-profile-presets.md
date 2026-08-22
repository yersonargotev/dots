# Keep capability Tags atomic and Profiles convenient

The repository used broad `web` and `mobile` Tags to select several unrelated
skills and agent-specific integrations at once. That preserved convenient
workbench setup, but it prevented operators from expressing narrower intent and
made a Tag larger than the smallest capability that could be installed or stop
being managed independently.

We define each current Tag as one independently meaningful capability. The
`web` Profile is the ordered preset `playwright`, `frontend-design`,
`vercel-web-skills`, `claude-chrome-devtools`, `codex-chrome-devtools`, and
`opencode-chrome-devtools`. The `mobile` Profile is the ordered preset
`dart-skills`, `flutter-skills`, `android-skills`, `claude-dart-mcp`,
`codex-dart-mcp`, `antigravity-dart-mcp`, and `vscode-mobile`.

The Profile names remain the convenient way to request the complete workbench.
They own no surface: the same ordered current Tags supplied explicitly produce
the same Selected Surface on Darwin and Linux. The two-step Claude Chrome
DevTools marketplace/plugin flow remains one Tag because both steps form one
usable outcome. Each agent-specific Dart or Chrome DevTools integration stays
separate so selecting one consumer does not configure another.

`adaptive-theme` and `codegraph` remain single current global opt-ins. Their
cross-cutting behavior applies at supported consumer seams, including source
overrides only when the corresponding Managed Entry is selected. Splitting
either global preference into consumer-specific variants would multiply intent
without creating independently meaningful operator choices.

The former broad `web` and `mobile` Tags become hidden legacy compatibility
aliases whose ordered replacements exactly match their Profiles. They own no
Dependency Set, Managed Entry, or Provisioner directly. Normal selection and
newly persisted intent use only current Tags, while explicit legacy input keeps
the previous Selected Surface during the announced transition.

**Considered options**: Keeping broad Tags was rejected because Profiles already
provide grouping convenience and broad Tags prevent independent selection.
Creating separate coarse `web-skills`, `browser-tools`, or `mobile-tools` Tags
was rejected because those groups still combine capabilities that have distinct
consumer ownership. Splitting global opt-ins by consumer was rejected because
it would make one preference require several coordinated selections.

**Consequences**: The normal Install Catalog exposes the complete atomic
inventory and hides `web` and `mobile` as legacy aliases. Repository Profile
parity tests protect the behavior-preserving presets on both supported operating
systems. The aliases remain for this slice and are removed only through a
separately visible follow-up after the compatibility release cycle; that future
change must update migration guidance and cannot infer authorization to remove
Managed Configuration or reverse Provisioner effects.

This decision supersedes ADR 0008's temporary rejection of splitting the Web
workbench, ADR 0009's broad Mobile Tag placement, and the broad Web/Mobile
portion of ADR 0013. Their capability boundaries and provisioning mechanisms
remain valid.
