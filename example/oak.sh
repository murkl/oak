# The product's library, loaded in front of every script of every module. What
# both modules mean by a tux tree is said once, here.

export TUX_ID=tux

tux_release() { printf '%s/etc/os-release' "$TUX_TARGET"; }

# The shell's own exit code is whoever typed in it, not a failure of the row.
tux_shell() {
    cd "$TUX_TARGET" || return 1
    bash || true
}

# ─── The YAML ───────────────────────────────────────────────────────────────

prefill_hostname() { echo tuxbox; }

value_desktop() { cat "${TUX_TARGET}/etc/desktop" 2>/dev/null || echo none; }
