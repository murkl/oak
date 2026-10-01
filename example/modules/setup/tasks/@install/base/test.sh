grep -q "^$TUX_HOST$" "$TUX_TARGET/etc/hostname"
grep -qx "ID=$TUX_ID" "$(tux_release)"
