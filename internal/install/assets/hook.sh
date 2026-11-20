#!/bin/dash
# Argus status hook. Written by `argus install`; do not edit.
# Rules: shell builtins only, nothing on stdout, never fails the agent.
[ -n "$ARGUS_ID" ] || exit 0
case "$ARGUS_ID" in *[!a-z0-9]*) exit 0 ;; esac
f="__STATE_DIR__/$ARGUS_ID"
cur=
[ -r "$f" ] && read -r cur < "$f"
case "$1" in
  start)  new=idle ;;
  prompt) new=working ;;
  perm)   new=waiting ;;
  notify) [ "$cur" = working ] || exit 0; new=waiting ;;
  tool)   [ "$cur" = waiting ] || exit 0; new=working ;;
  stop)   new=done ;;
  end)    new=ended ;;
  *)      exit 0 ;;
esac
[ "$new" = "$cur" ] && exit 0
printf '%s\n' "$new" > "$f" 2>/dev/null
exit 0
