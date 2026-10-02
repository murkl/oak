# No shebang, no set -e, no error handling: Oak runs this with oak.sh and an
# ERR trap, so a command that fails fails the task.
mkdir -p "$TUX_TARGET/etc" "$TUX_TARGET/home/$TUX_USER"
