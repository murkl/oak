# How this task says it took. Being called test.sh is the whole declaration, the
# same way being called task.sh is — nothing in the yaml beside it points here.
#
# The guard comes first, exactly as it does in a task: a simulated run wrote
# nothing, so without it every test here would fail for the one reason that is
# not a fault.
simulating && return 0

# It reads and it says. Nothing it does may leave a mark on the machine: it runs
# on a system halfway through being built, and a test that changes anything is a
# step nobody listed.
test -d "$TUX_TARGET/etc"
test -d "$TUX_TARGET/home/$TUX_USER"
