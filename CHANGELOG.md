# Changelog

What each release changed, newest first. Written by the run that publishes it, out of what landed on `main` since the release before — never by hand. The version is what `oak --version` answers and what a product pins itself to; what moves which number is the promise in the **[README](docs/README.md#1-get-oak)**.

## 0.5.0 - 2026-09-19

- A release is what lands on `main`: the version, the changelog and the tag are written by the run rather than typed at a desk
- A password the machine already has is asked once rather than twice — `existing: true` on a secret. The repeat catches a typo in a password being chosen, which nothing else would; one that already exists is refused within seconds by whatever it is handed to, which says more than a second box can
- A `confirm:` or `report:` naming a variable its module does not declare no longer loads. The name was filled in with nothing, and a warning that has quietly stopped naming the disk reads exactly like a finished sentence
- `--inspect` fails on a translation naming other `{{VAR}}` than the string it came from. Which order they appear in is the translator's to choose; which ones appear is not

## 0.4.1 - 2026-09-18

- The welcome page carries the product's address as writing alone — the code that stood beside the languages is gone

## 0.4.0 - 2026-09-18

- A product may say where the rest of it is — `url:` — and the welcome page carries it: written out, and drawn beside the languages as a code to scan
- The welcome page is laid out in the golden ratio, the words and the languages against the code
- A block laid beside another stands in a column of its own rather than against the longest line in it, so the code on a report page keeps its place
- The block cells every picture is drawn from are checked against a console font like every other mark, which the mark over a finished run never was

## 0.3.3 - 2026-09-18

- A changelog, and a release page made out of the section for the version it publishes
- The run warns when work has landed with no section open for it, or when a change wrote nothing into the one that is - without stopping either
- `make tag` reads the version out of the changelog rather than being handed one

## 0.3.2 - 2026-09-18

- The release page says how to check a download and what a version promises a product
- The checksum GitHub publishes replaces the checksum file a release used to carry
- The banner reads its accent out of the product rather than being drawn against a copy of it
- `make secrets-check` scans the repository, so CI and a desk run the same command

## 0.3.1 - 2026-09-18

- A list too long for one screen opens a box that narrows it
- The screenshots come out of a storyboard, every run of it started with `--debug`
- One pipeline per push: CodeQL moved into CI instead of racing it

## 0.3.0 - 2026-09-17

- A module keeps what it requires in its declaration, and the settings page can start a run over
- A module says what one run of it is called
- A secret is left off the settings page, where no row could open on it
- Backspace deletes wherever something is being typed
- `make tag` and `make version-check` refuse a release tag the binary does not answer to

## 0.2.0 - 2026-09-12

- A module says which machines it belongs on
- A failure closes only on purpose

## 0.1.0 - 2026-09-11

- First release: one binary that asks, keeps the answers, runs the shell and reports where it stopped
- A product is an `oak.yaml` with a folder of modules beside the binary
- Tasks laid out by stage, hooks, tests beside the work, answers that survive a run
- `--debug`, `--version` and `--module=<id>`, and nothing else on the command line
- The example product, the reference and the screenshots the README is made of
