#!/bin/dash
# Argus status hook. Written by `argus install`; do not edit.
# Rules: shell builtins only, nothing on stdout, never fails the agent.
# The state file holds two lines: the state and the agent's conversation id.
[ -n "$ARGUS_ID" ] || exit 0
case "$ARGUS_ID" in *[!a-z0-9]*) exit 0 ;; esac
f="__STATE_DIR__/$ARGUS_ID"
cur= sid=
[ -r "$f" ] && { read -r cur; read -r sid; } < "$f"
case "$1" in
  start)
    new=idle
    # The event's JSON (one line) carries the conversation id, which changes
    # on /resume and /clear. Only this event reads stdin.
    IFS= read -r line
    s=${line#*'"session_id":"'}
    [ "$s" = "$line" ] && s=${line#*'"session_id": "'}
    s=${s%%'"'*}
    case "$s" in '' | *[!A-Za-z0-9_-]*) ;; *) sid=$s ;; esac
    ;;
  prompt) new=working ;;
  perm)   new=waiting ;;
  notify) [ "$cur" = working ] || exit 0; new=waiting ;;
  tool)   [ "$cur" = waiting ] || exit 0; new=working ;;
  stop)   new=done ;;
  end)    new=ended ;;
  *)      exit 0 ;;
esac
[ "$1" != start ] && [ "$new" = "$cur" ] && exit 0
printf '%s\n%s\n' "$new" "$sid" > "$f" 2>/dev/null
exit 0
