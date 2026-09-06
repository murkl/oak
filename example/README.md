# The example product

A whole Oak product, small enough to read in one sitting: an `oak.yaml`, two modules beside it, and six shell scripts between them. It is what the screenshots in the [README](../docs/README.md) are taken from.

Nothing here touches the machine. **Tux Setup** asks what an installer asks — a hostname, a user, a desktop — and builds a small system tree in `./tux` out of the answers. **Tux Recovery** is a second whole program from the same binary: it checks that the tree is there.

```
oak.yaml                              the product: name, colour, version, wordmark
modules/setup/setup.yaml              what it asks, and the order its work happens in
modules/setup/tasks/target/           make the folder                  stage: prepare
modules/setup/tasks/base/             hostname and os-release          stage: install
modules/setup/tasks/desktop/          only when a desktop was chosen   stage: install
modules/setup/tasks/user/             the home folder, and the report  stage: finish
modules/recovery/recovery.yaml        the second module
modules/recovery/tasks/verify/        is the tree still there
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
