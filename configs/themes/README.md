# Carbonfox palette assets

The Carbonfox adapters in this repository are based on the canonical
[carbonfox.lua](https://github.com/EdenEast/nightfox.nvim/blob/4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a/lua/nightfox/palette/carbonfox.lua)
palette and generated extras from
EdenEast/nightfox.nvim@4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a.
Nightfox is MIT-licensed; the required notice is retained in
[LICENSE-nightfox](LICENSE-nightfox).

carbonfox.tmTheme preserves Nightfox's generated TextMate scope mappings.
Its stale Catppuccin metadata was corrected, and three hard-coded Catppuccin
colors left by the upstream generator were mapped to the corresponding
Carbonfox semantic roles. The generated white selection background was also
mapped to Carbonfox sel0 so selected text retains useful contrast.
