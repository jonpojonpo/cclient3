# cclient3 — Multi-Provider Terminal AI Agent
# Generate the demo GIF: vhs demo.tape
# Requires: cclient3 binary built, ANTHROPIC_API_KEY set

Output demo.gif

Require cclient3

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Padding 16
Set Theme "Dracula"
Set TypingSpeed 35ms
Set WindowBar Colorful
Set WindowBarSize 40
Set CursorBlink false

# --- Launch cclient3 ---
Hide
Env ANTHROPIC_API_KEY "${ANTHROPIC_API_KEY}"
Show

Type "cclient3"
Enter
Sleep 2s

# --- Show /help to display all commands ---
Type "/help"
Enter
Sleep 3s

# --- List registered providers ---
Type "/providers"
Enter
Sleep 2s

# --- Ask the agent something that fans out to parallel sub-agents ---
Type@80ms "Survey this repo: spawn one sub-agent to summarise the Go packages and another to review the README, then combine their findings."
Sleep 600ms
Enter

# Sub-agents stream into their own tabs — watch the tab bar
Sleep 15s

# --- Switch back to Chat tab ---
Type "/tab 1"
Enter
Sleep 1s

# --- Normal chat continues ---
Type@60ms "Now give me a one-paragraph executive summary"
Enter
Sleep 6s

# --- Clean exit ---
Ctrl+C
Sleep 500ms
