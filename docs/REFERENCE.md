# Reference

Everything a product may declare. Nothing here is compiled into Oak: a different `oak.yaml` with a different set of modules beside it is a different program out of the same binary.

Two rules run through the whole file:

- **`title:` is what a person reads. `name:` only ever names a variable.** A module, a task and a preset are named by their folder or their place, so none carries an id
- **Nothing is written down twice.** Which modules there are is the folders under `modules/`; which tasks there are, and what phase each runs in, is the folders under `tasks/`

## The product — `oak.yaml`

Sits beside the binary and holds what no module can answer for its neighbours. Every key is optional.

```yaml
title: Tux Linux
version: 1.0.0
accent: "#8fbcbb"
logo: |
  A product driven by

  ████████ ██    ██ ██   ██
```

| Key | Description |
| --- | --- |
| `title` | The product's name, over the pages drawn before a module is opened |
| `version` | What this build of the product is called, in the corner of every page. Left out, no version is shown |
| `accent` | `#rrggbb`. The one colour the interface is built from |
| `logo` | The wordmark. Everything above the first blank line is a dim eyebrow over it |

`version` is the product's own. Oak's own is what `--version` answers — `oak-0.1.0`, name and version as one word, the way a release names its files — and what the splash signs off with under the wordmark. It is never shown as though it belonged to the product.

## A module

One folder. Only the declaration has to be there — a module turns a part of the program off by leaving a file out.

| Path | Description |
| --- | --- |
| `module.yaml` | The declaration: what the module is, what it asks, and the order its work happens in |
| `module.sh` | Sourced in front of everything this module runs |
| `tasks/@<stage>/<task>/task.yaml` | What a task is |
| `tasks/@<stage>/<task>/task.sh` | What it does, where its yaml does not say so itself |
| `tasks/@<stage>/<task>/test.sh` | How to tell that it took, likewise. Optional — see [Testing the work](#testing-the-work) |
| `hooks/@<hook>/<step>/hook.yaml` | A moment Oak runs itself — see [Hooks](#hooks) |
| `hooks/@<hook>/<step>/hook.sh` | What that step does, where its yaml does not say so itself |
| `locales/<code>.po` | One catalog per language |

The work and the hooks are two folders, not one, because they answer to different things: a task is the module's own work — listed, ordered, guarded — while a hook is the module's answer to a question Oak asks at a moment of its own choosing. Every file inside says which of the two it is, so neither is ever read as the other.

The folder name is the module's identity: what `oak --module=<name>` opens, and what its files are called — `setup` writes `setup.conf` and `setup.log`. Everything inside it has the name Oak knows it by, so nothing points at anything.

### The declaration

```yaml
title: Tux Setup                         # the module's name, where it is talked about
action: Set up                           # optional: the word on the row that opens it
description: Set a machine up for Tux.   # shown where the modules are offered
stages: [prepare, install]               # the phases the work happens in, in order
                                         # — each a folder under tasks/

confirm: |                               # the last thing shown before anything changes
  {{TUX_HOST}} will be set up in {{TUX_TARGET}}.

console: Run ./oak --module=setup to start it again.  # optional: read on the way out
language: TUX_LOCALE                     # optional: ties the interface language to one answer
```

| Key | Description |
| --- | --- |
| `title` | **Required.** What the module is called, wherever the interface talks about it rather than starts it: the sentence over its settings, the last warning, the clock while it runs |
| `stages` | **Required.** The phases the work happens in, in order. Each is a folder under `tasks/`, marked — `tasks/@install/` — and the name written here carries no `@` of its own |
| `action` | The word on the row that **opens** it — on the page asking which module, and again on the menu. A row is pressed, so it says what will happen rather than what this is called. Left out, the row falls back on the title |
| `description` | One sentence, read on the page that offers the modules |
| `confirm` | The last thing shown before the first task. `{{VAR}}` is filled in from the answers |
| `console` | Read on the terminal on the way out, where the machine keeps running |
| `language` | Names a variable whose answer also settles the interface language. `de_DE` is matched to German |
| `presets` | See [Presets](#presets) |
| `variables` | See [Questions](#questions) |

## Questions

One entry under `variables:` is one question and one environment variable.

```yaml
variables:
  - name: TUX_HOST
    title: Hostname
    description: What the machine calls itself on the network.
    group: System
    required: true
    pattern: '^[a-z][a-z0-9-]*$'
    error: Lower case letters, digits and - only.
```

What is drawn follows from the declaration — there is no switch for it:

| Declaration | What is drawn |
| --- | --- |
| nothing further | A text box |
| `values: [btrfs, ext4]` | A list |
| `command: timedatectl list-timezones` | A list, built from what the command printed |
| `type: bool` | Yes / No, in the interface's language |
| `type: secret` | A password field, asked twice, never written to disk |

| Field | Description |
| --- | --- |
| `name` | **Required.** The environment variable a script reads |
| `title` | **Required.** The question |
| `description` | What this value is for, read above the question |
| `group` | The heading this row and the ones after it sit under, on the settings page |
| `required` | Required and unanswered is what makes Oak ask |
| `default` | The answer to start from. Any scalar: `true`, `8`, `pc105` |
| `prefill` | Shell that prints a suggestion into the box. A suggestion is not an answer, so it does not stop Oak asking |
| `apply` | Shell run when the answer takes effect — see below |
| `first` | Asked before everything else — see below |
| `free` | Label of a text box under a list, for a value the list only suggests |
| `pattern` | A regular expression the answer has to match |
| `error` | What a wrong answer is told. Left out, Oak names the rule that was broken |
| `conditions` | See [Conditions](#conditions) |

`true` and `false` are shown as Yes and No wherever they appear, so `values: [auto, true, false]` is a boolean with a third option.

**A secret** is the one required value that does not hold up the rest of the program. It is asked for immediately before the run that needs it, used, and forgotten — never written to the answer file or the log.

**`apply:`** is for an answer that changes the machine the program is running on rather than the one being worked on — `apply: loadkeys "$TUX_KEYMAP"`. It runs the moment the answer is given, and again at startup for an answer this run already had. A failure is logged as a warning and the answer still stands.

**`first: true`** puts a question before the network screen, the preflight and the presets, so a password can be typed on a keyboard layout that has already been settled. Use it sparingly: it is asked before the check that decides whether this machine can be worked on at all.

**A `command:`** may return a value and its display text on one line, separated by a **tab**. Everything before the tab is stored, everything after it is shown:

```bash
lsblk -dn -o PATH,SIZE,MODEL | awk '{printf "%s\t%s  %s %s\n", $1, $1, $2, $3}'
# /dev/nvme0n1<TAB>/dev/nvme0n1  1.8T WD Black
```

An empty value before the tab is a real answer ("no variant", "the default"), not a blank line to be dropped.

## Conditions

One condition, or a list where every one must hold:

```yaml
conditions:
  - TUX_DESKTOP != none
  - TUX_DRIVER == nvidia
```

`VAR == value` and `VAR != value`, and nothing else. Deliberately not an expression language: the two forms cover every guard an installer needs, and they are checked against the declared variables when the module loads — so a renamed variable is an error at startup rather than a task that silently never runs.

There is no `or`. A row that applies under two unrelated conditions is written as two rows.

## Tasks

One folder under the stage it runs in: `tasks/@<stage>/<task>/`. The stage folders are the phases `module.yaml` listed, marked with `@` the way the hooks are — the mark says the folder is a **when**, and the folder inside it a **what**.

```
tasks/
  @prepare/
    partition/
  @install/
    base/
    desktop/
```

```yaml
title: Install the graphics driver   # the line shown while the user waits
needs: [desktop-gnome]               # ordered after these, within the same stage
conditions:                          # every one must hold, or the task is skipped
  - TUX_DRIVER != none
```

The task folder's name is its identity — what another task's `needs:` points at — and the stage folder above it is when it runs. Moving a task to another phase is moving the folder, and nothing inside it can say one thing while it sits in another.

What it does is the `task.sh` beside its yaml, or the `script:` in it:

```yaml
title: Enable 32-bit support
script: |                            # shell, for a step short enough to read here
  sed -i '/\[multilib\]/,+1s/^#//' /etc/pacman.conf
  pacman -Sy --noconfirm
```

| Written as | What runs |
| --- | --- |
| nothing | The `task.sh` in the same folder |
| `script:` with shell in it | That shell |
| `script: ./install.sh` | That file, relative to this `task.yaml` |

A `script:` **and** a `task.sh` is two answers to the same question and is refused, as is neither. Shell written in the yaml has no file for a failure to point at, so what a failure names is the command and the exit code rather than a file and a line.

Seven more keys change what a task **is** rather than what it does:

| Key | Description |
| --- | --- |
| `asks: VAR` | The run pauses to ask for that value first, for something not knowable before the work started. The variable must have a fixed set of answers and must not be a secret |
| `confirm:` | Asked as a yes/no before it runs. Declining skips it and the run carries on |
| `default: no` | That yes/no opens on No instead of Yes |
| `report:` | The run stops on a page of its own once this task has finished. The first paragraph is the headline; `{{VAR}}` is filled in. Where anything has been tested, the page also says how many passed |
| `shows: VAR` | Puts that answer on the report page as a scannable code, and under it as text |
| `quits: true` | The program does not return after this task — a reboot |
| `tty: true` | The interface steps aside and hands the script the whole terminal |

`shows:` is for a value meant to be used on a different machine than the one displaying it. It is read back from the answer file after the task has run, which is also how the task puts it there:

```bash
printf "MY_LINK='%s'\n" "$url" >>"$MODULE_CONF"
```

### Testing the work

A task may also say how to tell that it took. That is `test:`, found exactly the way `script:` is — written here, naming a file, or simply lying beside it as `test.sh`. It is **optional**: a task that says nothing about it is simply never tested.

```yaml
title: Install the boot loader
test: |
  arch-chroot "$MNT" bootctl is-installed | grep -q yes
```

| Written as | What runs |
| --- | --- |
| nothing | The `test.sh` in the same folder, where there is one |
| `test:` with shell in it | That shell |
| `test: ./verify.sh` | That file, relative to this `task.yaml` |
| neither, and no `test.sh` | Nothing. The task is simply not tested |

It runs immediately after the work, on the machine that work was done to, and it has one rule: **it reads and it says nothing else**. It runs on a system halfway through being built, and a test that changes anything is a step nobody listed. What it prints goes to the log.

Its exit status is the answer, the same way a task's own is: a command that failed, or whatever it handed back at the end.

A script can say no without any command having failed — `return 1` and a guard that does not fire both look like that, and the trap sees neither — and the report still names the line: what comes back then is the last line the script itself was on. A command that really did fail is named where that command is, which for a function out of `module.sh` is the line inside `module.sh`.

A run started with `--debug` runs them like any other. A test is a module's own script under the same contract as the work: it is handed `DEBUG` and decides for itself what a run that changed nothing has to say. So it opens with the same guard its task does.

A test that fails does not fail the run. The work said it worked, and something looking at the machine afterwards disagreed — that is a thing to read, not a reason to abandon an installation that is already on the disk.

So the tally is read where somebody is actually looking: on every page a `report:` stops the run on, and again under the line that says the run is over. A run whose last offer is a restart is a run most people never see the end of, which is why it is not only said there.

Where something disagreed, the next page is the list of what did — offered **once**, at the first of those stops, and not at all where everything passed. Choosing a row opens the same file-and-line report a failed task gets: the module, the task, the file and line in its `test.sh`, the command and what the tool said. Leaving the list carries the run on into whatever it was going to offer next.

Whether any of this happens at all is one switch in the settings, on unless somebody turns it off. It is Oak's own answer rather than a module's — what a task tests is the module's business, whether anything is tested is not — so it is kept in `oak.conf` and holds for every module beside it. The switch is offered only where the module has something to test.

### The order

```
tasks/
  @prepare/
    partition/
    format/        needs: [partition]
  @install/
    base/
    desktop/       needs: [base]
    graphics/      needs: [base]
```

```mermaid
flowchart LR
    subgraph A["tasks/@prepare"]
        direction TB
        P["partition"] --> F["format"]
    end
    subgraph B["tasks/@install"]
        direction TB
        BS["base"] --> DE["desktop"]
        BS --> GR["graphics"]
    end
    A --> B
```

- A task runs after every task of an earlier stage
- Within its stage, it runs after whatever it named in `needs:`

Two tasks that neither a stage nor a `needs` separates are independent. Their order is stable from run to run, but it is not something to build on — the folder name is the task's identity, not a way to steer the order.

`needs:` names a task **in the same stage**. A name no task anywhere answers to is refused at startup, and so is a cycle, which is reported as the ring it goes round: `a → b → c → a`. A name belonging to another stage says nothing the stages have not already said, so it is dropped with a warning rather than refused — `--inspect` reports it and the log records it.

A folder under `tasks/` whose name is not a declared stage is refused, and so is a task lying straight under `tasks/` with no stage folder over it: work that never runs because a stage is misspelled is the one mistake nothing else would ever show. A folder named after one of the hooks is refused too, and told which half of the tree it belongs in.

## Presets

Pages of starting points, offered once on a machine that has answered nothing yet. A preset is a set of answers, not a mode: every value it sets can still be changed afterwards.

```yaml
presets:
  - title: Setup
    description: What kind of system to install.
    options:
      - title: Desktop
        description: A full desktop.
        values:
          TUX_DESKTOP: gnome

      - title: Online                  # a starting point fetched rather than written out
        description: Take the answers somebody shared.
        asks: TUX_CONFIG_SOURCE        # the one question this row asks
        apply: ./tasks/@finish/share/import.sh  # shell turning that answer into more answers
```

A preset is named by its title and nothing else. Nothing points at one, so there is no id to keep unique.

## Hooks

Seven moments belong to Oak rather than to the module. Each is a folder under `hooks/`, named with the `@` the hook itself carries, holding one folder per step with a `hook.yaml` and — where the shell is too long for the yaml — a `hook.sh` in it.

Oak decides when a hook runs. A module that fills none of them has no `hooks/` folder at all, and simply does not get those parts of the program.

| Hook | Description |
| --- | --- |
| `@preflight` | Can this machine be worked on at all. A hard stop, run before everything except the `first` questions. What it writes to stderr is what the user reads |
| `@online` | Is there internet. Without it the network screen never appears |
| `@wlan-device` | Which wireless device to use |
| `@wlan-networks` | The networks in range, one SSID per line |
| `@wlan-connect` | Join one, with `WLAN_DEVICE`, `WLAN_SSID` and `WLAN_PASSPHRASE` in the environment |
| `@restart` | Shut this machine down and start it again |
| `@shutdown` | Switch it off |

```
hooks/@preflight/root/hook.yaml       title: Running as root
hooks/@preflight/firmware/hook.yaml   title: UEFI, Secure Boot off
hooks/@online/https/hook.yaml         title: Reach the network
```

Every step of a hook runs in order, one process each, and `@preflight` stops at the first that says no — so a check that is really four checks is written as four, each with a name of its own. `needs:` orders them the way it orders a stage. What they print comes back as one answer, which is how `@wlan-networks` hands over a list.

Running them one at a time is what makes a mistake in one findable. A hook is a module's own shell, and a typo in it is an authoring bug like any other: the failure names the module, the hook, the step, the file, the line and the command, exactly as a failed task does.

A step is written like any other unit — a `title:`, what it `needs:`, and its `hook.sh` or `script:` — but it is run at a fixed moment rather than listed, offered, reported on or tested afterwards, so `test:`, `conditions:`, `asks:`, `confirm:`, `default:`, `report:`, `shows:`, `quits:` and `tty:` are refused: a line that can never take effect is a line somebody will read as though it could.

The title of a `@preflight` step is read: it is what the failure page names when that check is the one that said no. Everywhere else it is what the file calls itself, and nothing more.

`@restart` and `@shutdown` turn leaving the interface into a choice rather than a plain exit: a module that fills them is saying the machine booted specifically to run it. A module with neither exits like any ordinary program.

## What a script receives

Every declared variable under its own name, answered or not, and two names of Oak's own:

| Variable | Description |
| --- | --- |
| `MODULE_CONF` | The answer file. Also how a script answers a question back: append `KEY='value'` to it |
| `DEBUG` | `true` when the run was started with `--debug`. Absent otherwise |

That is the whole list, and it is meant to stay that way. Anything else a script needs it works out for itself — its own folder, for instance, is where `module.sh` was sourced from:

```bash
MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
```

Scripts run in a shell that already carries an `ERR` trap and, where the module has one, `module.sh`. They need no preamble: no shebang, no `set -e`, no error handling. **Any non-zero status is a failure** — a command that failed anywhere in the script, or whatever the script itself hands back at the end. If one fails, the unit fails, the run stops there, and the page it stops on is the one a finished run stops on under the other mark. Behind it is the module, the unit, the file, the line, the command, the exit code and what the tool said — the same page every failure in the program opens on.

**`module.sh` is sourced in front of everything** — every task, every test, every step of every hook, and every piece of shell the yaml writes for a `command:`, a `prefill:`, an `apply:`, a `script:` or a `test:`. So a function defined there is called by name from the yaml:

```yaml
apply: load_console_keyboard
command: list_locales
```

```yaml
# hooks/@online/https/hook.yaml
title: Reach the network
script: is_online
```

It is **loaded, not run**. A lookup in it that tries one thing and falls back to another is ordinary shell and is nobody's failure, so it is sourced outside the trap: what it recovers from is never reported as the failure of the unit that was about to run. The one thing that is its own failure is a shell that will not load at all, and that fails the unit with whatever it said on the way out.

Four rules, and no more:

- **Change nothing while simulating.** `simulating && return 0` before the first line that touches anything, with `simulating()` defined in your `module.sh` as `[ "$DEBUG" = true ]`
- **Never end on a command that can fail unless you mean it.** A script answers with its exit status, so `[ "$X" = y ] && do_it` as the last line fails it when the test is false. End on the real work, on an `if` block or on an `echo` — and where you do mean it, `exit 1` and `return 1` both say so
- **Ask nothing.** Every question is declared in the yaml, unless the task declares `tty: true`
- **Print nothing for a person to read.** stdout and stderr go to the log; the screen shows the task's name

A `test:` keeps all four and adds a fifth: **change nothing at all**. It reads a machine somebody is still installing onto, and its exit code is the whole of what it has to say. The first rule is the one that matters most there — a simulated run wrote nothing, so a test that does not open with the same guard its task does will fail for the one reason that is not a fault.

`command:`, `prefill:`, `apply:`, `script:` and `test:` each accept either shell or a file. A single line starting with `./` or `../` names a file, relative to the folder of the yaml it was written in; anything else is the shell itself.

## Files it writes

Beside wherever the program was started, never inside a module — which may be a read-only medium:

| File | Description |
| --- | --- |
| `oak.conf` | What Oak keeps across every module: `OAK_LANG`, the language, and `OAK_VALIDATE`, whether a run checks its own work |
| `<module>.conf` | Every answer, as `KEY='value'`. Plain shell, editable by hand |
| `<module>.log` | Oak's own progress plus every line every script printed |

A second module writes its own pair beside the first, so two started from the same folder never collide.

## Keys

Five keys, three meanings, the same on every page. Long lists narrow with `/`.

| Key | Meaning |
| --- | --- |
| `enter` | Confirm |
| `esc`, `backspace` | Back |
| `q`, `ctrl+c` | Ask to leave |

**Note:** _Arrow keys only move a cursor, since an arrow key is also what a mouse wheel sends._

## Checking a product

Two options that answer on stdout instead of drawing anything. They read the product beside the binary, the same one a run would open, and `--module=` narrows both to one of its modules:

```
oak --inspect                    # load the product beside the binary, and report
oak --inspect --module=setup     # just that one module
oak --strings --module=setup     # write that module's translation template
```

`--inspect` loads a product exactly as a run does — every task ordered, every condition resolved — and prints what it found. **This is the check to put in a build script.** Two of its lines are about the gap between the yaml and the shell, in opposite directions:

| Line | Meaning |
| --- | --- |
| `unread` | A question asked where no task that reads the answer can run. **This fails the check** — it is the one authoring mistake a module's shape does not rule out on its own |
| `unset` | A name in capitals the module's shell reads that nothing here answers. A description, not a verdict — `$HOME` and `$PATH` belong on that line |
| `needs` | A `needs:` naming a task in another stage. Also a description: the stages already put the two in that order |

`unset` is where a name that used to arrive and no longer does becomes visible. In shell an unset name is an empty string rather than an error, so nothing else would ever say so.

## Translations

The source string is the key. A line of yaml says `Your name` and a catalog answers with `Dein Name`; a catalog with nothing to say about a string leaves the English standing, which is what makes a half-finished translation useful from its first line.

```
./oak --strings --module=setup > modules/setup/locales/setup.pot
cp modules/setup/locales/setup.pot modules/setup/locales/fr.po
```

Two catalogs are merged: Oak's own, compiled into the binary, and the module's under `locales/`. A catalog names its own language as the translation of `English`, and that is what the language picker lists — so a language is always shown in its own words.

The language is chosen on the welcome page every run opens on, and can be changed in the settings afterwards. It opens on whatever `oak.conf` last recorded, or on whatever `LC_ALL`, `LC_MESSAGES` or `LANG` comes closest to. It never reaches a script: what a script does is the same in every language.

**Note:** _The welcome page itself is the one page no catalog is read for. It is drawn before a language has been settled, so it stays in plain English whatever the last run chose._

**Note:** _The Linux virtual console holds at most 512 glyphs. A product that runs there before any desktop exists is safe with ASCII and the Latin-1 letters, and not with Greek, Cyrillic or anything written in a script of its own._
