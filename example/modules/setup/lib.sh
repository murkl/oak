# Sourced in front of every script of this module. It is for what several
# scripts have to agree about, and nothing else.

# Whether this run only pretends to work. Oak sets DEBUG for every script when
# it was started with --debug and leaves it unset otherwise, so a script that
# changes anything opens by reading it.
simulating() { [ "$DEBUG" = true ]; }
