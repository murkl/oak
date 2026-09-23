# Loaded in front of everything any module of this product runs, before the
# module's own module.sh. It is for what the modules have to agree about with
# each other: Tux Setup builds a tree that Tux Recovery then checks, and both
# have to mean the same thing by one.
#
# Neither module has a module.sh: a module that shares nothing between its own
# scripts simply leaves the file out.

# What a tux tree is called in the file that names it. Read by the modules'
# scripts, which shellcheck reads one at a time, so it looks unused here.
# shellcheck disable=SC2034
TUX_ID=tux

# That file, in whichever tree the answers point at.
tux_release() { printf '%s/etc/os-release' "$TUX_TARGET"; }
