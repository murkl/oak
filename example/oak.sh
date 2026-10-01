# The product's library: loaded in front of every script any of its modules
# runs, and the one place they share code. Tux Setup builds a tree that Tux
# Recovery then checks, and both have to mean the same thing by one.

# What a tux tree is called in the file that names it. Exported, because it is
# a value the scripts share rather than one this file uses.
export TUX_ID=tux

# That file, in whichever tree the answers point at.
tux_release() { printf '%s/etc/os-release' "$TUX_TARGET"; }

# A shell in that tree, until whoever opened it types exit. The shell's own exit
# code is theirs, not a failure of the row that opened it.
tux_shell() {
    cd "$TUX_TARGET" || return 1
    bash || true
}
