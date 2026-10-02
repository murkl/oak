# The Example Product

A whole Oak product, small enough to read in one sitting. The screenshots in the **[README](../docs/README.md)** are taken from it. Nothing here touches the machine: **Tux Setup** builds a small system tree in `./tux`, **Tux Recovery** checks it.

```
oak.yaml                               the product: name, colour, version, wordmark
oak.sh                                 the library both modules share
modules/setup/module.yaml              what it asks, its stages, its rules
modules/setup/actions/writable/        start-if: can this folder be written to
modules/setup/actions/note/            on-settings: a page and a script
modules/setup/actions/shell/           on-success: a shell
modules/setup/tasks/@prepare/target/   a task and its test.sh
modules/setup/tasks/@install/base/     hostname and os-release
modules/setup/tasks/@install/desktop/  only when a desktop was chosen
modules/setup/tasks/@finish/user/      the home folder, and the report
modules/recovery/                      the second module
```

## Running It

From the repository root:

```
make run                 # asks which module to open
make run MODULE=setup    # opens Tux Setup outright
make run ARGS=--debug    # shows the run, starts nothing
make inspect             # loads it as a run does, and reports
```

A run leaves `setup.conf`, `setup.log` and `./tux` beside this file. Delete `setup.conf` to be asked everything again.

**[➜ Reference](../docs/REFERENCE.md)** for every key.
