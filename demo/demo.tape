# The README demo. Regenerate from the repo root with:  vhs demo/demo.tape
# Uses stand-in apps (demo/make-fixtures.sh) and --dry-run, so nothing on
# the recording machine is touched or shown.

Output assets/demo-cli.gif
Output assets/demo-cli.mp4

Require go

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 20
Set Width 1400
Set Height 860
Set Padding 28
Set Margin 48
Set MarginFill "demo/backdrop.png"
Set BorderRadius 14
Set WindowBar Colorful
Set Theme "Catppuccin Mocha"
Set Framerate 30
Set TypingSpeed 70ms

Hide
Type 'export D="$TMPDIR/twin-demo" PS1="\[\e[38;5;141m\]❯\[\e[0m\] " && go build -o "$D/bin/twin" ./cmd/twin && demo/make-fixtures.sh "$D/apps" && export PATH="$D/bin:$PATH" TWIN_DIRS="$D/apps" && clear'
Enter
Sleep 5s
Show

Sleep 600ms
Type "twin --dry-run"
Sleep 500ms
Enter
Sleep 1.5s

# Browse, then filter to Slack.
Down@300ms 3
Sleep 500ms
Type "/"
Sleep 300ms
Type@140ms "sla"
Sleep 800ms
Enter
Sleep 400ms
Enter
Sleep 1s

# Name the profile and watch the preview update live.
Type@160ms "Work"
Sleep 1.2s

# Optionally start the twin with a copy of the original's data.
Ctrl+D
Sleep 1.4s
Enter

# Clone, then admire the result.
Sleep 6s
Sleep 2.5s
