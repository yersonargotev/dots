-- Portable Rex shortcuts. Unlisted client defaults remain available.
-- No Herdr compatibility prefix; Rex and Ghostty are independent terminals.
-- The theme is imported/selected by the client; see README.md.

rex.bind("cmd+t", "client.tab.new")
rex.bind("cmd+shift+h", "client.tab.previous")
rex.bind("cmd+shift+l", "client.tab.next")
rex.bind("cmd+shift+w", "client.tab.close")
rex.bind("cmd+1", "client.tab.goto", { index = 1 })
rex.bind("cmd+2", "client.tab.goto", { index = 2 })
rex.bind("cmd+3", "client.tab.goto", { index = 3 })
rex.bind("cmd+4", "client.tab.goto", { index = 4 })
rex.bind("cmd+5", "client.tab.goto", { index = 5 })
rex.bind("cmd+6", "client.tab.goto", { index = 6 })
rex.bind("cmd+7", "client.tab.goto", { index = 7 })
rex.bind("cmd+8", "client.tab.goto", { index = 8 })
rex.bind("cmd+9", "client.tab.goto", { index = -1 })

rex.bind("cmd+shift+k", "session.previous")
rex.bind("cmd+shift+j", "session.next")
rex.bind("cmd+k", "session.switch")
rex.bind("cmd+shift+n", "session.new")

rex.bind("cmd+d", "pane.split", { direction = "right" })
rex.bind("cmd+shift+d", "pane.split", { direction = "down" })
rex.bind("cmd+shift+enter", "pane.zoom")
rex.bind("cmd+w", "pane.close")

-- Stop at pane edges instead of wrapping to the opposite side.
rex.bind("ctrl+alt+h", "pane.focus", { direction = "left", wrap = false })
rex.bind("ctrl+alt+j", "pane.focus", { direction = "down", wrap = false })
rex.bind("ctrl+alt+k", "pane.focus", { direction = "up", wrap = false })
rex.bind("ctrl+alt+l", "pane.focus", { direction = "right", wrap = false })

-- Cmd+R enters an exclusive mode; Escape returns to normal input.
rex.mode("resize", { exclusive = true })
rex.bind("cmd+r", "client.mode.enter", { name = "resize" })
rex.bind("resize/h", "pane.resize", { direction = "left", amount = 5 })
rex.bind("resize/j", "pane.resize", { direction = "down", amount = 5 })
rex.bind("resize/k", "pane.resize", { direction = "up", amount = 5 })
rex.bind("resize/l", "pane.resize", { direction = "right", amount = 5 })
rex.bind("resize/escape", "client.mode.exit")
