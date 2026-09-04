<h1 align="center">Oak</h1>

<div align="center">

<p><strong>One binary that turns YAML and shell scripts into a guided installer.</strong></p>

<p>
  <img src="https://img.shields.io/github/v/release/murkl/oak?style=for-the-badge&label=RELEASE&color=8fbcbb">
  <img src="https://img.shields.io/badge/License-GPL_3.0-blue?style=for-the-badge">
</p>

</div>

Every installer is the same program twice: a menu, a set of questions, somewhere to keep the answers, a list of steps and a way to say which one broke. Oak is that program, written once. What is left for you is the part that is actually yours.

- **Declare, do not draw.** A question is a few lines of YAML. Oak decides what it looks like
- **Shell stays shell.** A step is a script that does one thing. No framework, no API, no bindings
- **Nothing is lost.** Every answer is written down the moment it is given, so an interrupted run picks up where it left off
- **Reproducible.** That answer file is plain shell. Copy it to the next machine and every question it answers is skipped
- **One file to ship.** A static binary with no dependencies, your declaration and your scripts beside it

**Note:** _Oak knows nothing about any operating system. Not a disk, not a package, not a bootloader. That half is yours, and it stays in shell where you can read it._

## How it Works

Oak looks next to itself, and nowhere else:

```
oak                 the binary
oak.yaml            what the product is called and what it looks like
modules/setup/      one module: setup.yaml and the folders beside it
modules/repair/     another one
```

A **module** is one whole program: what it asks, in what order it works, and the shell it runs. A **product** is the modules a binary is shipped with, under one name and one colour.

| You want to | Write |
| --- | --- |
| Ask a question | A few lines under `variables:` |
| Do something | A folder with a `task.yaml` and a `task.sh` in it |
| Add a second program | Another folder under `modules/` |
| Rename or recolour the whole thing | Three keys in `oak.yaml` |

Nothing lists the tasks anywhere. The folder is the list, and the order comes out of the stages they name and what each says it needs.

## Quick Start

### 1. Get Oak

```
curl -Lo oak https://github.com/murkl/oak/releases/latest/download/oak-linux-amd64
chmod +x oak
```

### 2. Say what the Product is

`oak.yaml`, beside the binary:

```
name: Demo
version: 0.1.0
accent: "#8fbcbb"
```

### 3. Write a Module

`modules/hello/hello.yaml`:

```
title: Demo Setup
description: Write a greeting to a file.
stages: [write]

variables:
  - name: DEMO_NAME
    title: Your name
    required: true
```

`modules/hello/tasks/greet/task.yaml`:

```
name: Write the greeting
stage: write
```

`modules/hello/tasks/greet/task.sh`:

```
echo "Hello, ${DEMO_NAME}!" >./greeting.txt
```

### 4. Run it

```
./oak
```

Oak asks for a language, then for the one question that is required and still unanswered, then runs the task. A single module is opened on the way in rather than offered; a second folder under `modules/` is what makes that a page. The answers land in `hello.conf`, everything the script printed in `hello.log`.

**Note:** _A working version of this is in **[example](../example)**. `oak --inspect` reads a product the way a run does and reports what it found, which is the check to put in a build script._

## What the Interface Does

Each page appears only when it has something to show:

| Page | When |
| --- | --- |
| Language | More than one is available |
| What to do | The product offers more than one module |
| Network | The module defines an `online.sh` hook |
| The check | The module defines a `preflight.sh` hook. A failure here is a hard stop |
| Presets | The module declares starting points, and this machine has answered nothing yet |
| The questions | One per page, for what is required, meaningful and still unanswered |
| Settings | Every answer on one page, once nothing is left to ask |
| Running | The tasks, filling in from the top |
| A failure | Which script, which line, which command, which exit code, and where the rest is logged |

Four keys, one meaning each, on every page:

| Key | Meaning |
| --- | --- |
| `enter` | Confirm |
| `esc`, `backspace` | Back |
| `q`, `ctrl+c` | Ask to leave |

**Note:** _Arrow keys only move a cursor, since an arrow key is also what a mouse wheel sends._

## Everything Else

**[➜ See Reference](REFERENCE.md)** for the whole of what a product may declare: questions, presets, tasks, conditions, hooks, the script contract and translations.

## Built with Oak

**[Arch OS](https://github.com/murkl/arch-os)** is a reproducible Arch Linux installation: an installer and a recovery, both modules, on one bootable image. It is the reference implementation and a good place to read a real product.

## Development

```
make run          # Oak against the example product
make check        # everything that has to pass before a commit
make release      # bin/oak-linux-amd64, plus its checksum
```

Install the required packages:

```
sudo pacman -S --needed go make shellcheck shfmt staticcheck yamllint actionlint gettext
```

**[➜ See Contributing](CONTRIBUTING.md)**

## License

GPL-3.0. See **[LICENSE](../LICENSE)**.

## Credits

Many thanks for these projects and the people behind them!

- Bubble Tea by charm
- gettext
