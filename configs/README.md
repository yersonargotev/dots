# Configuration sources

`dots.yaml` defines the offered Managed Entries. Files outside that manifest
are not selected or installed by the current Profiles or Tags.

The retired tmux, Zellij, tuicr, Warp and Zed directories retain only sources
that earlier manifests installed as symlinks (including their source override
variants). They remain at their original paths so refreshing the Source of Truth
does not break existing workstation links. The shared TextMate Carbonfox asset
also remains in use by bat. Retired copied configuration and integrations have
been removed from the Source of Truth.

These inactive files do not restore retired capabilities to the catalog. Their
eventual removal requires a separately authorized transition for existing
symlinks; manifest evolution alone does not authorize that retirement.
