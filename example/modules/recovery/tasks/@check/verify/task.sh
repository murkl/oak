# This task only reads, so its yaml says it simulates itself: a run started
# with --debug checks the tree exactly as an ordinary one does.

test -f "$TUX_TARGET/etc/hostname"
grep -qx "ID=$TUX_ID" "$(tux_release)"

# And whatever the tree said it was built with is still there to find.
if [ "$TUX_DESKTOP" != none ]; then
    test -f "$TUX_TARGET/etc/desktop"
fi
