[ -w . ] && return 0
echo "This folder cannot be written to. Start Tux Setup somewhere you own." >&2
exit 1
