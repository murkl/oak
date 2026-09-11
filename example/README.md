# The example product

A whole Oak product, small enough to read in one sitting: an `oak.yaml`, two modules beside it, and a handful of shell scripts between them. It is what the screenshots in the [README](../docs/README.md) are taken from.

Nothing here touches the machine. **Tux Setup** asks what an installer asks — a hostname, a user, a desktop — and builds a small system tree in `./tux` out of the answers. **Tux Recovery** is a second whole program from the same binary: it checks that the tree is there.

```
oak.yaml                                    the product: name, colour, version, wordmark
modules/setup/module.yaml                   what it asks, and the order its work happens in
modules/setup/module.sh                     what more than one of its scripts agrees about
modules/setup/hooks/@preflight/writable/    a hook: can this folder be written to at all
modules/setup/tasks/@prepare/target/        make the folder, and a test.sh beside it
modules/setup/tasks/@install/base/          hostname and os-release, tested inline
modules/setup/tasks/@install/desktop/       only when a desktop was chosen, and inline
modules/setup/tasks/@finish/user/           the home folder, and the report
modules/recovery/module.yaml                the second module
modules/recovery/tasks/@check/verify/       is the tree still there
```

A folder under `tasks/` is one phase of the run, marked `@` and named in `module.yaml`; a folder inside it is one task. `hooks/` is the other half: one folder per moment the runtime runs shell of its own accord, with the steps in it declared in `hook.yaml`.

## Running it

From the repository root:

```
make run                     # asks which module to open
make run MODULE=setup        # opens Tux Setup outright
make run ARGS=--debug        # every script is handed DEBUG=true and changes nothing
make inspect                 # loads it the way a run does, and reports what it found
```

Every task here says how to tell that it took, in a `test:` or a `test.sh`. Those run after the work, read the tree and change nothing, and the run ends by saying how many of them passed. Each opens with the same `simulating` guard its task does, so `make run ARGS=--debug` reports them all as passed rather than all as failed. The switch that turns them off for good is in the settings.

A run leaves `setup.conf`, `setup.log` and `./tux` beside this file. Delete `setup.conf` to be asked everything again.

Every key any of these files uses is in the **[Reference](../docs/REFERENCE.md)**.
