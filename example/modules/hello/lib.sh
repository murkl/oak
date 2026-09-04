# Sourced in front of every script of this module. It is for what several
# scripts have to agree about, and nothing else.

# Whether this run only pretends to work. Oak hands every script DEBUG, and a
# script that changes anything opens by reading it.
simulating() { [ "$DEBUG" = true ]; }
