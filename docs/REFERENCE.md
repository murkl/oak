# Reference

Everything a product may declare. A different `oak.yaml` with different modules beside it is a different program out of the same binary.

- `title:` is what a person reads, `name:` only ever names a variable
- Nothing is written down twice: the modules are the folders under `modules/`, the tasks the folders under `tasks/`

## The Product

```
oak                 the binary
oak.yaml            the product
oak.sh              optional: the one library every script gets
modules/<module>/   one folder per module
```

### `oak.yaml`

```yaml
title: Tux Linux
version: 1.0.0
accent: "#8fbcbb"
status:
  check: check_online()
  pass: Online
  fail: Offline
logo: |
  A product driven by

  ████████ ██    ██ ██   ██
icon: icon_tux()
```

| Key | Description |
| --- | --- |
| `title` | The product's name, over every page |
| `version` | This build of the product, under the wordmark on the way in and in the corner of every page. `--version` answers it after Oak's own |
| `accent` | `#rrggbb`, the one colour the interface is built from |
| `logo` | The wordmark. Above the first blank line a dim eyebrow |
| `icon` | Over every module's menu, centred above its rows, in the accent. Default: a tick |
| `status` | One line about the machine in the header, see [Status](#status) |

- `logo` and `icon` are a picture as written, or `name()`: a function of `oak.sh` that prints it, run once at startup
- An `icon` five rows tall stands to the menu's rows in the golden ratio, as the tick does
- A picture that cannot be drawn stops the start

**Note:** _A terminal of sixteen colours, such as the Linux console, shows every colour in the slot of its hue: green, yellow, red, blue, cyan, white and grey (8). A heading is not bold there, since a console draws bold as the bright slot. The accent takes the stock colour nearest it. A product that paints the console's palette paints those slots._

### `oak.sh`

The one place scripts share code. Loaded in front of every task, test and action, and of every function the yaml calls. It defines functions and exports constants, and works nothing out while it loads.

```bash
export TUX_ROOT=/mnt

tux_release() { printf '%s/etc/os-release' "$TUX_TARGET"; }

icon_tux() {
    cat <<'EOF'
 ▄█▄
▀▀ ▀▀
EOF
}
```

**Note:** _A `module.sh` in a module and an `actions/` beside `oak.yaml` are refused. What modules share is a function here._

### Shell in the YAML

A key that runs shell names it, in one of two forms:

| Form | Runs |
| --- | --- |
| `name()` | That function of `oak.sh`. Refused at startup where `oak.sh` has none by that name |
| `./file.sh` | That file, relative to the folder of the yaml |

Shell written into the yaml itself is refused: every line lives where a linter reads it and a failure can point at it. The keys are `check` of a status, and `options-from`, `value-from`, `prefill`, `apply` and `check` of a question. `logo` and `icon` take a function too.

- A function is named after the key that calls it: `options_disks()`, `prefill_hostname()`, `apply_keymap()`, `value_desktop()`, `check_online()`, `icon_tux()`

### Status

```yaml
status:
  check: check_online()   # exit 0 is yes
  every: 10            # seconds between two checks, default 10
  pass: Online
  fail: Offline
```

- A module's own `status:` replaces the product's
- Read again at once after an action ran
- `pass` and `fail` are translated through the module's catalog

## A Module

| Path | Description |
| --- | --- |
| `module.yaml` | What it is, asks and does |
| `tasks/@<stage>/<task>/task.yaml` | A task |
| `tasks/@<stage>/<task>/task.sh` | Its work |
| `tasks/@<stage>/<task>/test.sh` | Optional: how to tell it took |
| `actions/<action>/action.yaml` | An action |
| `actions/<action>/action.sh` | Its work |
| `locales/<code>.po` | One catalog per language |

The folder name is the module's identity: `oak --module=setup` opens it, and it writes `setup.conf` and `setup.log`.

### `module.yaml`

```yaml
title: Tux Setup
stages: [prepare, install]
confirm: true
language: TUX_LOCALE

rules:
  offer-if: [live-image]
  start-if: [root, internet]
  on-settings: [wifi]
  on-leave: [restart, shutdown]
  on-failure: [share-log]
  on-success: [shell]
```

| Key | Description |
| --- | --- |
| `title` | **Required.** The module's one name: its row, and the trail over every page |
| `stages` | **Required.** The phases of the work, in order. Each is a folder `tasks/@<stage>/` |
| `confirm` | `true` asks whether to start, after every password, before the work. Default `false` |
| `icon` | Its own icon, in place of the product's, see [`oak.yaml`](#oakyaml) |
| `language` | A variable whose answer also sets the interface language: `de_DE` is German |
| `rules` | Where its actions run, see [Rules](#rules) |
| `status` | Its own header line, in place of the product's |
| `presets` | See [Presets](#presets) |
| `variables` | See [Questions](#questions) |

### The Menu

- Its rows are **Start** and **Setup** in every module and every language. **Setup** opens every answer
- With `confirm: true` the last page before the work asks whether to start. It opens on No, and No goes back to the menu with every password forgotten

### Rules

Each rule names actions by their folder. An `-if` runs by itself and answers yes or no. An `on-` is a row offered at that place. A module and an action write their rules the same way, under `rules:`.

| Rule | In | Where |
| --- | --- | --- |
| `offer-if` | module, action | Before it is offered at all |
| `start-if` | module | Before the work. The first no stands a page in front of everything and is asked again every few seconds |
| `on-settings` | module | Rows at the top of the settings page |
| `on-leave` | module | Rows on the page every way out arrives at, above Oak's own **Exit** |
| `on-failure` | module | Rows under a run that failed |
| `on-failure` | action | The first of them this machine offers, opened where this action says no |
| `on-success` | module | Rows under a run that finished |

- A row list opens on **Continue**: an action is chosen on purpose
- A module with `on-leave` says the machine booted to run it, so leaving becomes a choice. `--kiosk` puts **Reset** where **Exit** is
- `offer-if` decides which modules a machine is offered: several are asked for under the wordmark, one is opened on the way in, none prints each module's `error` and stops. `--debug` offers everything

## Questions

One entry under `variables:` is one question and one environment variable, in `module.yaml` and `action.yaml` alike.

```yaml
variables:
  - name: TUX_DISK
    type: list
    title: Disk
    description: Everything on it is erased.
    group: Storage
    required: true
    options-from: options_disks()
    pattern: '^/dev/'
    error: Choose a disk that exists.
```

Every question names its type, and the type decides how it is drawn and which keys mean anything for it:

| `type` | Drawn as | Takes |
| --- | --- | --- |
| `text` | A text box | `default`, `prefill`, `pattern`, `value-from` |
| `bool` | Yes or No | `default`, `value-from` |
| `list` | A list | `options` or `options-from`, `default`, `prefill`, `pattern`, `filter` |
| `open-list` | A list, and a row for an answer of one's own | `options` or `options-from`, `default`, `prefill`, `pattern`, `filter` |
| `password` | A password that exists already, typed once | `check` |
| `new-password` | A password being chosen, typed twice | |
| `deferred` | A list, asked mid-run by the task that names it under `asks` | `options` or `options-from`, `filter` |

| Key | Description |
| --- | --- |
| `name` | **Required.** The variable a script reads |
| `type` | **Required.** One of the types above |
| `title` | **Required.** The question |
| `description` | What the value is for |
| `group` | Its heading on the settings page |
| `required` | Required and unanswered is what makes Oak ask |
| `default` | The answer to start from: `true`, `8`, `pc105` |
| `options` | The answers, written out |
| `options-from` | Shell that prints the answers. `value<TAB>label` stores the value and shows the label |
| `value-from` | Shell that prints the value instead of asking. Never asked, never on the settings page, never stored |
| `prefill` | Shell that prints a suggestion. Still asked |
| `apply` | Shell run as the answer takes effect on this machine, such as a keymap |
| `check` | Shell that tries a `password` before it is taken |
| `first` | Asked before everything else, `start-if` included |
| `filter` | The list's narrowing box: `collapsed`, behind `/`, or `open` |
| `pattern` | A regular expression the answer must match |
| `error` | What a wrong answer is told |
| `conditions` | When it is asked, see [Conditions](#conditions) |

- `true` and `false` read as Yes and No, so a `list` of `[auto, true, false]` is a bool with a third answer
- A password is asked right before the run, used and forgotten
- `value-from` is read when the module opens and whenever an answer changes
- An answer from `options-from` is held to its list again when the work is started, before any password. One the list no longer prints is asked again
- `apply` failing on an answer just given is a warning. At startup the answer is dropped and asked again

**Note:** _A key its type does not take is refused at startup, and so is `value-from` together with `prefill` or `first`. A deferred question takes no `first` or `group`, a password no `first`._

### Conditions

```yaml
conditions:
  - TUX_DESKTOP != none
  - TUX_DRIVER == nvidia
```

`VAR == value` and `VAR != value`, and every one must hold. There is no `or`: a row for two unrelated conditions is two rows. A name no variable answers is refused at startup.

### Placeholders

`{{VAR}}` is filled in from the answers in a task's `confirm` and `report`, and an action's `error` and `report`. A name the module does not declare is refused at startup, and a translation that drops or adds one fails `--inspect`.

## Tasks

A folder under its stage: `tasks/@<stage>/<task>/`. The `@` marks a **when**, the folder in it a **what**.

```yaml
title: Install the graphics driver
needs: [desktop]
conditions:
  - TUX_DRIVER != none
```

| Key | Description |
| --- | --- |
| `title` | **Required.** The line shown while it runs |
| `needs` | Tasks of its own stage it runs after |
| `conditions` | Every one must hold, or it is left out of the run |
| `asks` | A deferred question the run stops for first. A list that comes back empty skips the task |
| `confirm` | A yes or no before it runs, opening on No. No skips it |
| `yes-after` | Tasks before it: where one of them ran in this run, `confirm` opens on Yes |
| `report` | A page the run stops on afterwards. The first paragraph is the headline |
| `progress` | `true`: the last line it printed is shown under it while it runs |
| `simulates` | `true`: run under `--debug` too, reading `DEBUG` itself |
| `allow-failure` | `true`: its failure does not stop the run |

- A task runs after every task of an earlier stage, and within its stage after its `needs`
- A `needs` into another stage is dropped with a warning. An unknown name and a cycle are refused
- A folder under `tasks/` that is no declared stage is refused, and so is a task with no stage above it

### Tests

A `test.sh` beside `task.sh` runs right after the task, on the machine it worked on. It reads and changes nothing, and its exit status is the answer.

- A failed test does not stop the run
- Every page the run stops on counts them: `38 of 40 tests passed`
- Where a test or an `allow-failure` task failed, the finished run offers **Test results**: the list, each row opening the file, the line, the command and what it said
- **Verify steps** in the settings turns tests off for every module. It is kept in `oak.conf`

## Presets

Starting points, offered once on a machine that has answered nothing. A preset is a set of answers, not a mode.

```yaml
presets:
  - title: Desktop
    description: A full desktop.
    values:
      TUX_DESKTOP: gnome

  - title: Online
    description: Take the answers somebody shared.
    action: import
```

A preset writes its answers under `values` or names the action that fetches them, never both. The action's script appends them to `MODULE_CONF`.

## Actions

A script a module runs outside its work: `actions/<action>/action.yaml` and `action.sh`. The yaml says how it behaves, the script only does it: exit 0 is yes.

```yaml
title: Wireless network
description: Join a wireless network.
rules:
  offer-if: [wifi-card]
  on-failure: [wifi-passphrase]
variables:
  - name: WIFI_SSID
    type: list
    title: Network
    options-from: options_networks()
```

| Key | Description |
| --- | --- |
| `title` | **Required.** Its row, and the heading over its page |
| `description` | The sentence under its row. Choosing the row is the consent, so it says what leaves the machine |
| `error` | What a no means. **Required** under `offer-if` and `start-if` |
| `rules` | `offer-if` and `on-failure`, the same keys as a module's, see [Rules](#rules) |
| `variables` | Its questions before it runs, a page each: questions without `first`, `group` or `value-from` |
| `report`, `shows` | Its one page after it ran, and an answer drawn there as a code |
| `tty` | Its one page is the terminal itself |
| `simulates` | Run under `--debug` too |

- **One kind of page at most:** its questions, a report or the terminal. A flow of several kinds is several actions, chained by `on-failure`
- Run by itself, under an `-if`, an action is a question and has no page
- Its answer belongs to the session: never stored, never on the settings page
- `tty: true` hands the script the terminal outright, with a process group of its own

**Note:** _Refused at startup: an unknown name, an action nothing names, an action opened on its own failure, a ring of actions, a second kind of page, a page on an action run by itself, a check without `error`, a question under `on-leave`, and a rule only a module places: `start-if`, `on-settings`, `on-leave`, `on-success`._

## What a Script Receives

Every declared variable under its own name, and two of Oak's:

| Variable | Description |
| --- | --- |
| `MODULE_CONF` | The answer file. A script answers by appending `KEY='value'` |
| `DEBUG` | `true` under `--debug`, absent otherwise |

Scripts run with `oak.sh` loaded and an `ERR` trap: no shebang, no `set -e`, no error handling.

- **Any non-zero status is a failure**, a failed command or what the script hands back. `[ "$X" = y ] && do_it` as the last line fails where the test is false
- **Ask nothing.** Every question is in the yaml, unless the action has `tty: true`
- **Print nothing for a person.** stdout and stderr go to the log
- A `test.sh` also changes nothing

A failure names the module, the unit, the file, the line, the command, the exit code and what the tool said.

### Simulating

`--debug` shows a run without doing it. No task, test or action starts unless it declares `simulates: true`, and nothing under `start-if` is asked. What a question calls still runs, with `DEBUG=true`.

## Files It Writes

Beside wherever the program was started, never inside a module:

| File | Description |
| --- | --- |
| `oak.conf` | Oak's own: `OAK_LANG` and `OAK_VALIDATE` |
| `<module>.conf` | Every answer as `KEY='value'`, editable by hand. No password, no derived answer, no action's answer |
| `<module>.log` | Oak's progress and everything every script printed |

The last row of the settings page deletes the answer file, after a question that opens on No. The log stays.

## Keys

| Key | Meaning |
| --- | --- |
| `enter` | Confirm |
| `esc` | Back |
| `backspace` | Back, except in a text box |
| `q`, `ctrl+c` | Ask to leave |
| `/` | Narrow a long list |

**Note:** _Arrow keys only move a cursor. In a text box `q` and backspace are characters._

## Checking a Product

```
oak --inspect                  # load the product as a run does, and report
oak --inspect --module=setup   # one module
oak --strings --module=setup   # its translation template
oak --glyphs                   # every character the interface draws on a console
```

| `--inspect` line | Meaning |
| --- | --- |
| `unread` | A question no task that reads it can act on. **Fails the check** |
| `unset` | A name in capitals the shell reads that nothing answers |
| `needs` | A `needs` into another stage |
| `translation drops`, `adds` | A catalog naming other `{{VAR}}` than its source. **Fails the check** |

## Translations

The English string is the key. A catalog without a string leaves the English.

```
./oak --strings --module=setup > modules/setup/locales/setup.pot
cp modules/setup/locales/setup.pot modules/setup/locales/fr.po
```

- Oak's catalog and the module's are merged. A catalog names its language as the translation of `English`
- The language is chosen on the welcome page or with `--language=de`, and kept in `oak.conf`
- The welcome page is never translated: it is drawn before a language is settled

**Note:** _The Linux console holds 512 glyphs at most. ASCII and Latin-1 are safe. `oak --glyphs` prints everything Oak draws, so a build can hold its console font to it. `kbd`'s `default8x16` holds all of it._
