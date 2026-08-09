# Git configuration

`configs/git/loader.gitconfig` is installed as a dots-owned initial block in the
regular native `~/.gitconfig`. The block loads the portable
`configs/git/gitconfig` symlink at `~/.config/dots/git/gitconfig`, then the
established `~/.gitconfig.local` extension. Native content follows the block and
therefore has final precedence.

The native file remains writable by `git config --global` and other Git tooling
without changing the Installed Repository. Dots preserves comments and values
outside its exact marked block during install, update, migration, and uninstall.
Duplicate, incomplete, moved, or edited blocks are Conflicts and are not mutated.

Machine-specific configuration belongs in `~/.gitconfig.local`. Use
`gitconfig.local.example` as a starting point for the local file; never commit the
real local file.

## Classification from current machine evidence

The current global Git config was inspected by key name only, not by committing
private values. The resulting boundary is:

| Category | Examples | Repository decision |
| --- | --- | --- |
| Portable | `init.defaultBranch`, `pull.rebase`, `merge.conflictStyle=diff3` | Managed in `configs/git/gitconfig` and loaded first. |
| Local/private | `user.name`, `user.email`, signing keys, credential helpers | Kept in `~/.gitconfig.local`. |
| Generated | credential stores, tool caches, Git runtime state | Not managed by `dots`. |
| Machine-specific | work/client `includeIf` rules, absolute local paths, optional pager/tool wiring such as delta | Kept in `~/.gitconfig.local` or native content unless a later issue manages that tool explicitly. |

Git reads the three layers in this order:

1. portable baseline;
2. machine-local extension;
3. native additions after the dots block.

## Local extension point

The managed loader works when `~/.gitconfig.local` is absent. That is intentional:
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

The expected result is that `~/.gitconfig` is a regular file containing the
initial dots block and that `~/.config/dots/git/gitconfig` is the portable
symlink. Both the Dotfiles CLI target paths and the Go toolchain caches are
redirected into `$sandbox_root`; the maintainer's real home directory must not
be touched.
