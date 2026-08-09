-- Native regular-file entrypoint for dots-managed Neovim configuration.
local managed = vim.fn.expand("~/.config/dots/nvim")
vim.opt.runtimepath:prepend(managed)
dofile(managed .. "/init.lua")
