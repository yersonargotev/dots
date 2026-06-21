# Install Dart and Flutter agent skills through an explicit mobile profile

The user wants Dart and Flutter agent skills available on demand without making mobile development part of the default, desktop, web, or broad agent setup. We add a dedicated `mobile` Profile/tag for the mobile workbench and attach two skills.sh provisioners: `dart-lang/skills` and `flutter/skills`.

This keeps profile names aligned with user intent. Choosing `--profile mobile` means “prepare this machine/session for Dart and Flutter app work”. Choosing `--profile web` remains focused on frontend/browser tooling, choosing `--profile agents` remains focused on general agent setup, and choosing `--profile desktop` remains focused on terminal/editor/GUI machine configuration.

**Considered Options**: Adding Dart and Flutter skills to `agents` was rejected because it would make mobile-specific guidance part of every agent setup. Adding them to `web` was rejected because Flutter can target the web, but the requested capability is primarily mobile app development. Adding Flutter or Dart SDK package-manager dependencies was rejected for now because the request only asked to install the upstream skills; SDK installation can be added later if the profile is expanded from agent guidance into full mobile runtime/toolchain setup.

**Consequences**: Manifest profile selection now has a `mobile` tag. The mobile profile installs all upstream skills from `dart-lang/skills` and `flutter/skills` globally for Codex, Claude Code, and Antigravity through the pinned skills.sh CLI. Documentation and repository manifest tests must treat Dart and Flutter skills as mobile-scoped from this point forward. Initial implementation is tracked in [#149](https://github.com/yersonargotev/dots/issues/149).
