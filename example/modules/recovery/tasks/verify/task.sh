# Nothing here is guarded by simulating: this task only reads. A run started
# with --debug checks the tree exactly as an ordinary one does.

test -f "$TUX_TARGET/etc/hostname"
test -f "$TUX_TARGET/etc/os-release"
