test -f "$TUX_TARGET/etc/hostname"
grep -qx "ID=$TUX_ID" "$(tux_release)"

if [ "$TUX_DESKTOP" != none ]; then
    test -f "$TUX_TARGET/etc/desktop"
fi
