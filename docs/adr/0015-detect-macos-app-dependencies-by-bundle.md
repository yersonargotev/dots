# Detect macOS application Dependencies by bundle

Some Homebrew casks install graphical applications under `/Applications`
without placing their executable on `PATH`. A command-only Dependency probe can
therefore remain missing after its cask installs successfully, causing the
Dependency Plan and post-install gate to repeat an already completed action.
Ghostty is the first observed case.

Dependencies may declare an optional `darwin_app` bundle name, such as
`Ghostty.app`. On macOS, the command probe and application-bundle probe are
alternatives: either one satisfies the Dependency. Outside macOS,
`darwin_app` is ignored and normal command detection remains authoritative.
Application detection checks the selected user's `~/Applications` and the
system `/Applications`. The manifest accepts only a single `.app` bundle name,
not a path, so Install Manifest data cannot redirect the read-only probe outside
those reviewed roots.

The application lookup is an injected Dependency boundary, parallel to command
and font lookups. Check, plan, integrated install, standalone dependency
install, post-install verification, doctor, and Provisioner readiness reuse the
same result. Tests provide a sandbox lookup or a temporary `--home`; they never
depend on or mutate the operator's real Applications directories.

**Consequences**: macOS cask installations can converge without PATH shims or
ad hoc shell commands. The Install Manifest must explicitly opt each Dependency
into application detection, so an unrelated app bundle cannot satisfy a
command-only Dependency. App-bundle presence does not prove that the application
launches correctly; deeper health probes remain separate, tool-specific
diagnostics when a concrete need justifies them.
