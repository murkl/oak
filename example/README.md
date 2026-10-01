# The example product

A whole Oak product, small enough to read in one sitting: an `oak.yaml`, the `oak.sh` its modules share, two modules beside them, and a handful of shell scripts between them. It is what the screenshots in the [README](../docs/README.md) are taken from.

Nothing here touches the machine. **Tux Setup** asks what an installer asks — a hostname, a user, a desktop — and builds a small system tree in `./tux` out of the answers. **Tux Recovery** is a second whole program from the same binary: it checks that the tree is there, and works out for itself which desktop it was built with rather than asking.

```
oak.yaml                                    the product: name, colour, version, wordmark
oak.sh                                      the library: what the two modules agree about
modules/setup/module.yaml                   what it asks, the order its work happens in, its rules
modules/setup/actions/writable/             what the work starts if: can this folder be written to
modules/setup/actions/note/                 a row on the menu, with a page and a script
modules/setup/actions/shell/                what it offers once the run has finished: a shell
modules/setup/tasks/@prepare/target/        make the folder, and a test.sh beside it
modules/setup/tasks/@install/base/          hostname and os-release, and its test
modules/setup/tasks/@install/desktop/       only when a desktop was chosen
modules/setup/tasks/@finish/user/           the home folder, and the report
modules/recovery/module.yaml                the second module
modules/recovery/actions/shell/             the same shell, out of the same function in oak.sh
modules/recovery/tasks/@check/verify/       is the tree still there
```

A folder under `tasks/` is one phase of the run, marked `@` and named in `module.yaml`; a folder inside it is one task, doing its work in `task.sh`. `actions/` is the other half: one folder per script the module runs outside its work, declared in `action.yaml`, doing its work in `action.sh`, and named under `rules:` in `module.yaml`. What several scripts or both modules need is a function in `oak.sh`, which Oak loads in front of every one of them.

## Running it

From the repository root:

```
make run                     # asks which module to open
make run MODULE=setup        # opens Tux Setup outright
make run ARGS=--debug        # shows the run and starts nothing
make inspect                 # loads it the way a run does, and reports what it found
```

Every task of Tux Setup says how to tell that it took, in the `test.sh` beside it. Those run after the work, read the tree and change nothing, and the run ends by saying how many of them passed. A run under `--debug` starts neither, so none of them needs a guard. The switch that turns them off for good is in the settings.

A run leaves `setup.conf`, `setup.log` and `./tux` beside this file. Delete `setup.conf` to be asked everything again.

Every key any of these files uses is in the **[Reference](../docs/REFERENCE.md)**.
