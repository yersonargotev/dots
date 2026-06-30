local function dots_catppuccin_flavour()
  local marker = os.getenv("DOTS_ADAPTIVE_THEME_MARKER") or ((os.getenv("HOME") or "") .. "/.config/dots/adaptive-theme")
  if vim.fn.filereadable(marker) ~= 1 then
    return "mocha"
  end
  if vim.loop.os_uname().sysname ~= "Darwin" or vim.fn.executable("defaults") ~= 1 then
    return "mocha"
  end
  local out = vim.fn.system({ "defaults", "read", "-g", "AppleInterfaceStyle" })
  if vim.trim(out) == "" then
    return "latte"
  end
  return "mocha"
end

local flavour = dots_catppuccin_flavour()

return {
  {
    {
      "catppuccin/nvim",
      name = "catppuccin",
      priority = 1000,
      opts = {
        flavour = flavour, -- latte only when adaptive-theme + macOS light; mocha fallback
        transparent_background = true, -- disables setting the background color.
        term_colors = true, -- sets terminal colors (e.g. `g:terminal_color_0`)
      },
    },
    {
      "LazyVim/LazyVim",
      opts = {
        colorscheme = "catppuccin-" .. flavour,
      },
    },
  },
}
