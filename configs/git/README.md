# Git configuration

`configs/git/gitconfig` is the repository-managed Git configuration installed as
`~/.gitconfig`. It intentionally contains only portable defaults that are safe to
share across machines.

Machine-specific configuration belongs in `~/.gitconfig.local`, which is loaded
through the managed `[include]` entry. Use `gitconfig.local.example` as a starting
point for the local file; never commit the real local file.

## Classification from current machine evidence

The current global Git config was inspected by key name only, not by committing
private values. The resulting boundary is:

| Category | Examples | Repository decision |
| --- | --- | --- |
| Portable | `init.defaultBranch`, `pull.rebase`, `merge.conflictStyle=diff3` | Managed in `configs/git/gitconfig`. |
| Local/private | `user.name`, `user.email`, signing keys, credential helpers | Kept in `~/.gitconfig.local`. |
| Generated | credential stores, tool caches, Git runtime state | Not managed by `dots`. |
| Machine-specific | work/client `includeIf` rules, absolute local paths, optional pager/tool wiring such as delta | Kept in `~/.gitconfig.local` unless a later issue manages that tool explicitly. |

## Local extension point

The managed config works when `~/.gitconfig.local` is absent. That is intentional:
sandbox validation must not require private material.

For a real workstation only, create the local file deliberately and never
overwrite an existing one:

```bash
test -e ~/.gitconfig.local || install -m 600 configs/git/gitconfig.local.example ~/.gitconfig.local
```

Then edit `~/.gitconfig.local` with identity, signing, credentials, work-specific
includes, and optional local tooling. If the file already exists, inspect it
manually instead of replacing it; it may contain private identity, signing,
credential, or work configuration.

## Sandbox validation

Validate Git config changes with temporary directories, never the real `$HOME`:

```bash
sandbox_root="$(mktemp -d)"
sandbox_home="$sandbox_root/home"
sandbox_state="$sandbox_root/state"
sandbox_cache="$sandbox_root/cache"
sandbox_gopath="$sandbox_root/gopath"
sandbox_modcache="$sandbox_root/modcache"
mkdir -p "$sandbox_home" "$sandbox_state" "$sandbox_cache" "$sandbox_gopath" "$sandbox_modcache"

HOME="$sandbox_home" \
GOCACHE="$sandbox_cache" \
GOPATH="$sandbox_gopath" \
GOMODCACHE="$sandbox_modcache" \
XDG_CACHE_HOME="$sandbox_cache" \
go run ./cmd/dots install \
  --yes \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"

HOME="$sandbox_home" \
GOCACHE="$sandbox_cache" \
GOPATH="$sandbox_gopath" \
GOMODCACHE="$sandbox_modcache" \
XDG_CACHE_HOME="$sandbox_cache" \
go run ./cmd/dots status \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
```

The expected result is that `~/.gitconfig` is installed/aligned inside
`$sandbox_home`. Both the Dotfiles CLI target paths and the Go toolchain caches
are redirected into `$sandbox_root`; the maintainer's real home directory must
not be touched.
