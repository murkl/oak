# Reference

Everything a product may declare. Nothing here is compiled into Oak: a different `oak.yaml` with a different set of modules beside it is a different program out of the same binary.

## The Product

`oak.yaml` sits beside the binary and holds what no module can answer for its neighbours.

```
name: Demo               # over the pages drawn before a module is opened
version: 0.1.0           # what this build of the product is called
accent: "#8fbcbb"        # the one colour the interface is built from
logo: |                  # everything above the blank line is a dim eyebrow
  A product driven by

  ██████  ███████ ███    ███  █████    # the wordmark, in the accent colour
```

Every key is optional but `name`. Which modules exist is not written down: they are the folders under `modules/`, in name order. Adding one is a folder, removing one is deleting it.

**Note:** _`version` is the product's own, shown under the wordmark and in the corner of every page. Left out, Oak shows its own version instead, which is the right answer while a product is still being written._

## A Module

One folder. Only the declaration has to be there, so a module turns a part of the program off by leaving a file out.

```
<name>.yaml              what it is, what it asks, what order it runs in
tasks/<id>/task.yaml     where that step belongs
tasks/<id>/task.sh       what it does
hooks/<name>.sh          everything around the work itself, one script per hook
lib.sh                   sourced in front of every script of this module
locales/<code>.po        one catalog per language
```

The declaration is the one `.yaml` file at the top level, whatever it is called. Two of them in one folder is refused rather than resolved.

The folder name is the module's identity. It is what `oak <name>` opens, and what its answer file and log are called: `setup` writes `setup.conf` and `setup.log`.

### The Declaration

```
title: Demo Setup                       # the module's name on screen
description: Write a greeting to a file. # shown where the modules are offered
run: Setup                              # what one run of it is called

language: DEMO_LOCALE   # optional: ties the interface language to one answer

stages: [prepare, write]   # the phases the work happens in, in order

confirm: |                 # the last thing shown before anything changes
  A greeting for {{DEMO_NAME}} will be written to {{DEMO_TARGET}}.

console: Run ./oak hello to start it again.   # optional: shown on the way out
```

`confirm` is filled in from the answers, so it names the real target rather than describing things in the abstract.

**Note:** _`description` is what the module **is**. `run` is what one **run** of it is called, read out wherever the interface reports what is happening. A module that leaves `run` out defaults to "Installation"._

## Questions

One entry under `variables:` is one question and one environment variable.

```
variables:
  - name: DEMO_NAME
    title: Your name
    description: |
      Who the greeting is for.
    group: Greeting            # the heading this row and the ones after it sit under
    required: true             # required and unanswered is what makes Oak ask
    pattern: '^[A-Za-z][A-Za-z -]*$'
    error: Letters, spaces and - only.
```

What is drawn is decided by the declaration, not by a separate switch:

| Declaration | What is drawn |
| --- | --- |
| nothing further | A text box |
| `values: [btrfs, ext4]` | A list |
| `command: timedatectl list-timezones` | A list, built from what the command printed |
| `type: bool` | Yes / No, in the interface's language |
| `type: secret` | A password field, asked twice, never written to disk |

The other fields:

| Field | Description |
| --- | --- |
| `default` | Any scalar: `true`, `8`, `pc105` |
| `prefill` | Shell that prints a suggested answer into the box |
| `apply` | Shell run when the answer takes effect |
| `first` | Asked before everything else |
| `blind` | Opens with the filter box already up, for a question asked before any keyboard layout is settled. Only meaningful with `first` |
| `free` | Label of a text box under a list, for a value the list only suggests |
| `conditions` | See below |

**Note:** _`true` and `false` are shown as Yes and No wherever they appear, so `values: [auto, true, false]` is a boolean with a third option added._

A **secret** is the one required value that does not hold up the rest of the program while it is missing. It is asked for immediately before the run that needs it, used, and then forgotten.

**`apply`** is for an answer that changes the machine the program is running on, rather than the one being worked on:

```
apply: loadkeys "$DEMO_KEYMAP"
```

It runs the moment the answer is given, and again at startup for an answer this run already had, so a restart picks up where the last one left off. A failure here is logged as a warning and the answer still stands.

**`first: true`** puts a question before everything else: before the network screen, before the preflight, before the presets. It is what lets a password be typed on a keyboard layout that has already been settled rather than guessed.

**Note:** _Use `first` sparingly. Every question marked `first` is asked before the check that decides whether this machine can be worked on at all._

A command that provides options can return a value and its display text on one line, separated by a **tab**. Everything before the tab is stored, everything after it is shown:

```
lsblk -dn -o PATH,SIZE,MODEL | awk '{printf "%s\t%s  %s %s\n", $1, $1, $2, $3}'
# /dev/nvme0n1<TAB>/dev/nvme0n1  1.8T WD Black
```

**Note:** _An empty value before the tab is still a real answer ("no variant", "the default"), not a blank line to be ignored._

## Conditions

One condition, or a list where every one must hold:

```
conditions:
  - DEMO_DESKTOP != none
  - DEMO_DRIVER == nvidia
```

This is deliberately not an expression language. The two forms cover every guard an installer needs, and they are checked against the declared variables when the module loads, so a renamed variable is an error at startup rather than a task that silently never runs.

**Note:** _There is no `or`. A row that applies under two unrelated conditions is written as two rows._

## Tasks

A folder under `tasks/` with two files in it.

```
name: Install the graphics driver    # the line shown while the user waits
stage: desktop                       # which stage this runs in
needs: [desktop-gnome]               # ordered after these, within the same stage
conditions:                          # every one must hold, or the task is skipped
  - DEMO_DRIVER != none
```

Seven more keys change what a task **is** rather than what it does:

| Key | Description |
| --- | --- |
| `asks: VAR` | The run pauses to ask for that value before this task runs |
| `confirm:` | Asked as a yes/no before it runs. Declining skips it |
| `default: no` | That yes/no opens on No instead of Yes |
| `report:` | The run pauses on a page of its own once this task has finished |
| `shows: VAR` | Puts that answer on the page as a scannable code |
| `quits: true` | The program does not return after this task, a reboot |
| `tty: true` | The interface steps aside and hands the script the whole terminal |

**`asks`** is for a value that cannot be known before the work has started, such as which snapshot to roll back to once the disk holding them is open. The variable it names must have a fixed set of answers and must not be a secret.

**`report`** marks a milestone a list of task names cannot express: the work is done, and everything after it is optional. The first paragraph is the headline and `{{VAR}}` is filled in from the answers.

**`shows`** puts one answer on that page twice, large as a scannable code and printed underneath as plain text, for a value meant to be used on a different machine than the one displaying it. The value is read back from the answer file once the task has run, which is also how the task puts it there:

```
printf "MY_LINK='%s'\n" "$url" >>"$MODULE_CONF"
```

**Note:** _A cycle, an unknown stage or a `needs:` pointing at nothing produces an error at startup._

## Presets

Pages of starting points, offered once on a machine that has answered nothing yet. A preset is a set of answers, not a mode the program stays in: every value it sets can still be changed afterwards.

```
presets:
  - id: system
    title: Setup
    description: What kind of system to install.
    options:
      - id: desktop
        title: Desktop
        description: A full desktop.
        values:
          DEMO_DESKTOP: gnome

      - id: shared
        title: Online                 # a starting point fetched rather than written out here
        description: Take the answers somebody shared.
        asks: DEMO_CONFIG_SOURCE      # the one question this row asks
        apply: ./tasks/share/import.sh # shell that turns that answer into more answers
```

## Hooks

Bash scripts under `hooks/`, called by name. Nothing declares them: a script under one of these names **is** the declaration, and any other file name there is refused when the module loads.

| Hook | Description |
| --- | --- |
| `preflight.sh` | Can this machine be worked on at all. A hard stop, run before everything except the `first` questions |
| `online.sh` | Is there internet. Without it the network screen never appears |
| `wlan-device.sh` | Which wireless device to use |
| `wlan-networks.sh` | The networks in range, one SSID per line |
| `wlan-connect.sh` | Join one, with `WLAN_DEVICE`, `WLAN_SSID` and `WLAN_PASSPHRASE` in the environment |
| `restart.sh` | Shut this machine down and start it again |
| `shutdown.sh` | Switch it off |

Whatever the preflight writes to stderr is what the user reads.

`restart.sh` and `shutdown.sh` turn leaving the interface into a choice rather than a plain exit. A module that defines them is saying the machine booted specifically to run it, so every way out lands on a page offering restart or shutdown.

**Note:** _A module with neither hook exits like any ordinary program, which is right for something started from a shell the user is still sitting in._

## Shell Fields

`command:`, `prefill:` and `apply:` each accept either inline shell or a file. A single line starting with `./` or `../` names a file, anything else is the shell itself.

## What a Script Receives

Every declared variable under its own name, answered or not, plus:

| Variable | Description |
| --- | --- |
| `MODULE_DIR` | The module's folder, as an absolute path |
| `MODULE_CONF` | Its answer file |
| `MODULE_LOG` | Its log |
| `OAK_LANG` | The language currently on screen |
| `PRODUCT_VERSION` | The version the interface is showing |
| `DEBUG` | Whether this run is only simulating, see `--debug` |

Scripts are **sourced** into a shell that already has an `ERR` trap and, if the module declares one, `lib.sh`. They need no preamble: no shebang, no `set -e`, no error handling. If a command fails, the task fails and the user is shown the file, the line, the command and the exit code.

Four rules, and no more:

- **Change nothing while simulating.** `simulating && return 0` before the first line that touches anything, with `simulating()` defined in your `lib.sh` as `[ "$DEBUG" = true ]`
- **Never end on a command that can fail.** `[ "$X" = y ] && do_it` as the last line leaves the script's status at 1 when the test is false
- **Ask nothing.** Every question is declared in the yaml, unless the task declares `tty: true`
- **Print nothing for a person to read.** stdout and stderr go to the log, the screen shows the task's name

**Note:** _A script that simply ends because of a false test is not treated as a failure, except in a `tty: true` task, which has no wrapper around it and reports its own exit status directly._

## Files it Writes

Beside wherever the program was started, never inside a module, which may be a read-only medium:

```
./oak.conf         what Oak keeps across every module: the language
./setup.conf       every answer, as KEY='value': plain shell, editable by hand
./setup.log        everything: Oak's own progress plus every line a script printed
```

**Note:** _That is the `setup` module's pair. A second module writes its own beside it, so two started from the same folder never collide._

## The Command Line

```
oak [<module>] [--debug] [--version] [--inspect] [--strings]
```

| Flag | Description |
| --- | --- |
| `--debug` | Pass `DEBUG=true` to every script |
| `--version` | Print Oak's own version and exit |
| `--inspect` | Load the product and report what it holds, without running anything |
| `--strings` | Write one module's translation template to stdout |

Everything else on the line names the module to open. Whether a word names one depends on what is in `modules/` at that moment, which is what keeps the list of modules out of the binary.

- A module may be written as a bare word or with dashes: `oak setup` and `oak --setup` are the same request
- It may appear anywhere on the line
- A folder named after one of the four flags is refused at startup, since nothing could ever open it

**`--inspect`** loads a product exactly as a run does — every task ordered, every condition resolved — and prints what it found. It also refuses a question asked where no task that reads the answer can run, which is the one authoring mistake a module's shape does not rule out on its own. This is the check to put in a build script:

```
./oak --inspect
```

**Note:** _Nothing else on the command line is treated as an answer. Questions are answered in the interface and stored in the answer file beside it._

## Translations

The source string is the key. A line of yaml says `Your name` and a catalog answers with `Dein Name`. A catalog with nothing to say about a string leaves the English as it is, which is what makes a half-finished translation useful from its first line.

Two catalogs are merged: Oak's own, compiled into the binary, and the module's own under `locales/` beside its declaration.

```
./oak --strings setup > modules/setup/locales/setup.pot   # the template, out of the module
cp modules/setup/locales/setup.pot modules/setup/locales/fr.po
```

A catalog names its own language as the translation of `English`, and that is what the language picker lists, so a language is always shown in its own words.

The language is chosen on the first page of every run and can be changed afterwards in the settings. It opens on whatever `oak.conf` last recorded, or on whatever `LC_ALL`, `LC_MESSAGES` or `LANG` comes closest to.

**`language:`** in a module's declaration ties the interface language to one of its own answers. The value is matched against the catalogs the way a machine's own locale would be (`de_DE` is German), so a module that asks where a machine is has effectively also asked which language it speaks.

**Note:** _The Linux virtual console holds at most 512 glyphs. A product that runs there before any desktop exists is safe with ASCII and the Latin-1 letters, and not with Greek, Cyrillic or anything written in a script of its own._
