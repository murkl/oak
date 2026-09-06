simulating && return 0

echo "$TUX_HOST" >"$TUX_TARGET/etc/hostname"

{
    echo 'NAME="Tux Linux"'
    echo 'ID=tux'
} >"$TUX_TARGET/etc/os-release"
