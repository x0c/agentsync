# Run through scripts/record_demo.py to build and isolate the real CLI.
Output @OUTPUT@
Require agentsync

Set Shell "bash"
Set FontFamily "Menlo"
Set FontSize 18
Set Width 1040
Set Height 440
Set Padding 24
Set Theme "GitHub Dark"
Set Framerate 10
Set TypingSpeed 100ms
Set CursorBlink false
Env PATH @PATH@

Hide
Type "export PS1='$ '; clear; printf '# Real output excerpt: Claude Code + Codex\n\n'"
Enter
Sleep 500ms
Show
Sleep 500ms
Type "agentsync"
Sleep 500ms
Enter
Wait+Screen /mcp config/
Sleep 4s
