# The example product

A whole Oak product, small enough to read in one sitting: an `oak.yaml`, two modules beside it, and a handful of shell scripts between them. It is what the screenshots in the [README](../docs/README.md) are taken from.

Nothing here touches the machine. **Tux Setup** asks what an installer asks — a hostname, a user, a desktop — and builds a small system tree in `./tux` out of the answers. **Tux Recovery** is a second whole program from the same binary: it checks that the tree is there.

```
oak.yaml                                    the product: name, colour, version, wordmark
modules/setup/module.yaml                   what it asks, and the order its work happens in
modules/setup/module.sh                     what more than one of its scripts agrees about
modules/setup/tasks/@preflight/writable/    can this folder be written to at all
modules/setup/tasks/prepare/target/         make the folder
modules/setup/tasks/install/base/           hostname and os-release
modules/setup/tasks/install/desktop/        only when a desktop was chosen, and inline
modules/setup/tasks/finish/user/            the home folder, and the report
modules/recovery/module.yaml                the second module
modules/recovery/tasks/check/verify/        is the tree still there
```

## Running it

From the repository root:

```
make run                     # asks which module to open
make run MODULE=setup        # opens Tux Setup outright
make run ARGS=--debug        # every script is handed DEBUG=true and changes nothing
make inspect                 # loads it the way a run does, and reports what it found
```

A run leaves `setup.conf`, `setup.log` and `./tux` beside this file. Delete `setup.conf` to be asked everything again.

Every key any of these files uses is in the **[Reference](../docs/REFERENCE.md)**.
