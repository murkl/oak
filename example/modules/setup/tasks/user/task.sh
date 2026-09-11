simulating && return 0

printf 'export HOSTNAME=%s\n' "$TUX_HOST" >"$TUX_TARGET/home/$TUX_USER/.profile"
