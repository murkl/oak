echo "$TUX_HOST" >"$TUX_TARGET/etc/hostname"

{
    echo 'NAME="Tux Linux"'
    echo "ID=${TUX_ID}"
} >"$(tux_release)"
