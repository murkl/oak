# How this task says it took. Being called test.sh is the whole declaration, the
# same way being called task.sh is — nothing in the yaml beside it points here.
#
# It reads and it says. Nothing it does may leave a mark on the machine: it runs
# on a system halfway through being built, and a check that changes anything is
# a step nobody listed.
test -d "$TUX_TARGET/etc"
test -d "$TUX_TARGET/home/$TUX_USER"
