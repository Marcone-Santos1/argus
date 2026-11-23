#!/bin/dash
# Argus status hook. Written by `argus install`; do not edit.
# Rules: shell builtins only, nothing on stdout, never fails the agent.
# The state file holds the state, the agent's conversation id, how many
# subagents are running and "1" when the main turn ended while some still
# ran (background agents): the session stays working until the last one stops.
[ -n "$ARGUS_ID" ] || exit 0
case "$ARGUS_ID" in *[!a-z0-9]*) exit 0 ;; esac
f="__STATE_DIR__/$ARGUS_ID"
cur= sid= sub= held=
[ -r "$f" ] && { read -r cur; read -r sid; read -r sub; read -r held; } < "$f"
case "$sub" in '' | *[!0-9]*) sub=0 ;; esac
sub0=$sub held0=$held
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
    sub=0 held=
    ;;
  prompt)
    new=working
    # A count left by an interrupted subagent is cleared, unless background
    # agents were still running when the last turn ended.
    [ "$held" = 1 ] || sub=0
    held=
    ;;
  substart)
    sub=$((sub + 1)) new=$cur
    case "$cur" in idle | done | '') new=working ;; esac
    ;;
  substop)
    new=$cur
    [ "$sub" -gt 0 ] && sub=$((sub - 1))
    [ "$sub" = 0 ] && [ "$held" = 1 ] && { new=done held=; }
    ;;
  perm)   new=waiting ;;
  # Claude's "waiting for input" notice while only background agents run
  # is not a question.
  notify) [ "$cur" = working ] && [ "$held" != 1 ] || exit 0; new=waiting ;;
  tool)   [ "$cur" = waiting ] || exit 0; new=working ;;
  stop)
    if [ "$sub" -gt 0 ]; then new=working held=1; else new=done held=; fi
    ;;
  end)    new=ended sub=0 held= ;;
  *)      exit 0 ;;
esac
[ "$1" != start ] && [ "$new" = "$cur" ] && [ "$sub" = "$sub0" ] && [ "$held" = "$held0" ] && exit 0
printf '%s\n%s\n%s\n%s\n' "$new" "$sid" "$sub" "$held" > "$f" 2>/dev/null
exit 0
