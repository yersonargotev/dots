## Agent skills

### Issue tracker

Issues and PRDs are tracked in GitHub Issues for `yersonargotev/dots`. See `docs/agents/issue-tracker.md`.

### Triage labels

This repo uses the default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, and `wontfix`. See `docs/agents/triage-labels.md`.


### Dotfiles validation safety

Never validate dotfiles behavior against the user's real home configuration. Do not run `dots install` against the real `$HOME`, and do not write to live files such as `~/.zshrc`, `~/.tmux.conf`, `~/.gitconfig`, or `~/.config/starship.toml`. Use temporary directories plus explicit flags like `--home <tmp>`, `--source-root "$PWD"`, and `--state-root <tmp>` where supported.

### Pull requests

Use `.github/PULL_REQUEST_TEMPLATE.md` for PR descriptions. Include `Closes #<issue-number>`, exactly one `type:*` label, validation evidence, and the dotfiles safety checklist when config paths are involved.

### Domain docs

This repo uses a single-context domain documentation layout with `CONTEXT.md` at the root and ADRs under `docs/adr/`. See `docs/agents/domain.md`.
