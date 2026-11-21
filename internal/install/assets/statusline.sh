#!/bin/dash
# Argus usage line. Written by `argus install`; do not edit.
# Claude Code runs it for its status line. It saves the JSON it gets (rate
# limits, context) for the panel and prints nothing. Shell builtins only.
[ -n "$ARGUS_ID" ] || exit 0
case "$ARGUS_ID" in *[!a-z0-9]*) exit 0 ;; esac
js=
while IFS= read -r l || [ -n "$l" ]; do js="$js$l"; done
printf '%s\n' "$js" > "__STATE_DIR__/_usage/$ARGUS_ID" 2>/dev/null
exit 0
