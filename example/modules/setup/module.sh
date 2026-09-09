# Sourced in front of everything this module runs: every task, and every piece of
# shell its yaml writes for a list, a suggestion or a check. It is for what more
# than one of them has to agree about, and nothing else — a function named here
# can be called from module.yaml by name.

# Whether this run only pretends to work. Oak sets DEBUG for every script when
# it was started with --debug and leaves it unset otherwise, so a script that
# changes anything opens by reading it.
simulating() { [ "$DEBUG" = true ]; }
