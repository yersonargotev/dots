# Starship prompt configuration

`configs/starship/starship.toml` is the repository-managed Starship prompt
installed as `~/.config/starship.toml` (symlink strategy, `core` tag,
`darwin`+`linux`). It intentionally contains only portable defaults that are safe
to share across machines.

Shell integration stays out of this file. The prompt is loaded by a single
guarded hook in the Zsh slice (`configs/zsh/rc.d/post/40-tools.zsh`):

```sh
command -v starship >/dev/null 2>&1 && eval "$(starship init zsh)"
```

No prompt configuration lives in shell rc files — all of it is owned by this
Starship config.

## Prerequisite: a Nerd Font

The prompt uses Nerd Font glyphs for module icons, directory substitutions, and
the `character`/vim-mode symbols. A [Nerd Font](https://www.nerdfonts.com/) must
be installed and selected in your terminal for the prompt to render correctly.
This is a documented expectation, not a machine-specific path — the config makes
no assumption about which font or where it lives.

## Portability classification

The live workstation config was walked segment by segment before adoption. Every
segment was bucketed as portable, machine-specific, or private:

| Category | Examples | Repository decision |
| --- | --- | --- |
| Portable | `palette` + `[palettes.catppuccin_mocha]`, `format`, language/runtime modules (`nodejs`, `rust`, `golang`, `php`, `bun`, `java`, `c`, `zig`, `python`), `[character]` + vim symbols, `[fill]`, `[cmd_duration]`, `[time]`, `[directory]` substitutions, `add_newline`, `command_timeout` | Managed in `configs/starship/starship.toml`. |
| Machine-specific | absolute paths, hostnames, per-host module wiring, `[custom]` modules calling local-only binaries | None found in the live config. Nothing to gate. |
| Private | usernames, secrets, tokens, `env_var` modules surfacing private values | None found in the live config. The `[username]` module renders Starship's own `$user` variable at runtime — no name is committed. |

The live config contained no machine-specific or private segments, so the fully
portable config is adopted as the single source of truth with nothing excluded.

## Personal overrides — Starship has no native include

Unlike Git (`[include]`) or Zsh (sourcing a `.local` file), **Starship has no
native include or local-override mechanism**. Starship reads exactly one config
file (`$STARSHIP_CONFIG`, default `~/.config/starship.toml`); there is no
supported way to layer a `starship.local.toml` on top of it. Inventing one would
be silently ignored.

So personal or machine-specific tweaks are made by editing your own copy
directly, or by forking this repository. Because `dots` installs the config as a
**symlink**, editing `~/.config/starship.toml` edits the tracked file in place —
keep personal changes on a branch or fork rather than committing them to the
shared source. If a future need for private prompt segments appears (for example
a `[custom]` module calling a local-only script), the honest options are to fork
the config or point `$STARSHIP_CONFIG` at a separate machine-local file; this
repository does not ship a broken override file that Starship cannot read.

## Sandbox validation

Validate Starship config changes with temporary directories, never the real
`$HOME`:

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

The expected result is that `~/.config/starship.toml` is installed/aligned inside
`$sandbox_home` as a symlink to the repository source. The maintainer's real home
directory must not be touched.
