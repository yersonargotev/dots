local function dots_catppuccin_flavour()
  local helper = (os.getenv("HOME") or "") .. "/.config/dots/theme.sh"
  if vim.fn.filereadable(helper) ~= 1 or vim.fn.executable("sh") ~= 1 then
    return "mocha"
  end
  local out = vim.fn.system({ "sh", "-c", '. "$1" && dots_catppuccin_flavor', "sh", helper })
  if vim.v.shell_error == 0 and vim.trim(out) == "latte" then
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
