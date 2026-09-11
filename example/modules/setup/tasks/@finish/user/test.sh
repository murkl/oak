simulating && return 0

grep -q "^export HOSTNAME=$TUX_HOST$" "$TUX_TARGET/home/$TUX_USER/.profile"
