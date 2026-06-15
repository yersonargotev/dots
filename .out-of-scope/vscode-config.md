# VS Code configuration

`dots` does not manage Visual Studio Code as an editor. Neovim and Zed are the
editors in scope; VS Code config — including settings, keybindings, snippets,
and especially profiles — stays out of the repository-owned Source of Truth.

## Why this is out of scope

The original request (#52) wanted multiple VS Code profiles, each with its own
extension set, versioned in `dots`. The blocker is structural, not a matter of
effort.

VS Code stores the **Default** profile's authored files at fixed, known paths
(`User/settings.json`, `User/keybindings.json`, `User/snippets/`), which `dots`
could symlink like any other Managed Configuration. But **non-default profiles**
live under `User/profiles/<location>/`, where `<location>` is an id assigned by
VS Code when the profile is created — a hash for older profiles, a name slug for
newer ones. The name→id mapping lives in `globalStorage/storage.json`:

```jsonc
// User/globalStorage/storage.json → userDataProfiles
[
  { "location": "-7310d785", "name": "go" },
  { "location": "67cdc2eb",  "name": "js" },
  { "location": "agents",    "name": "Agents" }
]
```

Because `<location>` is machine-assigned, there is **no stable, portable symlink
target** for a named profile. Recreating the same profiles on another machine
produces different ids. The only ways to bridge that gap both fail the project's
principles:

- **Teaching `dots` to reconcile profiles** by reading and writing
  `globalStorage/storage.json` and symlinking into the generated `<location>`
  directories would make `dots` manage **generated application runtime state**
  and edit live storage — exactly what Managed Configuration must never own. It
  also exceeds the declarative Install Strategies (`symlink`, `template`,
  `copy`, no arbitrary shell execution).
- **Versioning `.code-profile` exports** and importing them is portable, but the
  import is a manual, interactive step in the VS Code UI. It does not fit the
  `symlink`/`copy` install model, so `dots` would be tracking files it cannot
  actually install — pretending to manage something it does not.

Managing only the Default profile would fit `dots` cleanly but cannot express
per-context toolchains (ESLint+Prettier vs Biome vs the Astral/Ruff stack),
which was the entire point of the request.

The maintainer also uses VS Code with a small, ad-hoc set of profiles and
prefers to keep that editor configured by hand rather than force it into the
dotfiles model. Portable, file-based editor config (Neovim, Zed) remains in
scope precisely because those editors store their authored configuration as
plain files at known paths.

## Prior requests

- #52 — "feat(config): migrate portable VS Code configuration"
