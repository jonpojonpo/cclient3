# cclient3 — Terminal AI Agent with Ensemble Mode
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

# --- Show /help to display all commands including ensemble ---
Type "/help"
Enter
Sleep 3s

# --- List available ensemble presets ---
Type "/ensembles"
Enter
Sleep 2s

# --- Show ensemble command usage ---
Type "/ensemble"
Enter
Sleep 2s

# --- Launch an ensemble discussion with default agents ---
Type@80ms "/ensemble What are the tradeoffs between microservices and monoliths for a startup?"
Sleep 600ms
Enter

# Agents respond in parallel — watch the Ensemble tab fill up
Sleep 10s

# --- User adds a follow-up message in the group chat ---
Type@60ms "Good points! What about developer experience and debugging complexity?"
Sleep 400ms
Enter
Sleep 8s

# --- End the ensemble session ---
Type "/done"
Enter
Sleep 2s

# --- Switch back to Chat tab ---
Type "/tab 1"
Enter
Sleep 1s

# --- Normal chat still works alongside ensemble ---
Type@60ms "Summarize the key takeaways from our ensemble discussion"
Enter
Sleep 6s

# --- Clean exit ---
Ctrl+C
Sleep 500ms
