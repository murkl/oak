# Sourced by Oak into a shell that already carries oak.sh and an ERR trap, so
# there is no shebang, no set -e and no error handling here: a command that
# fails fails the task, and the interface says which line it was. Nor is there
# anything about --debug: a simulated run does not start it at all.

mkdir -p "$TUX_TARGET/etc" "$TUX_TARGET/home/$TUX_USER"
