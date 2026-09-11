# Sourced by Oak into a shell that already carries module.sh and an ERR trap, so
# there is no shebang, no set -e and no error handling here: a command that
# fails fails the task, and the interface says which line it was.
simulating && return 0

mkdir -p "$TUX_TARGET/etc" "$TUX_TARGET/home/$TUX_USER"
