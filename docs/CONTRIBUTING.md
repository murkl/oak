# Contributing

Oak is one binary and nothing else ships. `main` is the only branch that lasts: every change branches off it and comes back as one commit, and a release is one more pull request.

## Workflow

```mermaid
flowchart LR
    M["main"] -->|branch off| B["feat/…"]
    B -->|pull request| C["Check"]
    C -->|squash merge| M2["main"]
    M2 --> P["Release pull request<br/>version · changelog"]
    P -->|merge| R["Release<br/>tag · binary · page"]

    style R fill:#8fbcbb,stroke:#8fbcbb,color:#2e3440
```

1. **Branch off `main`.** Name it after what it does: `feat/wireless-settings`, `fix/fuzzy-catalog`. Nothing reads the name
2. **Open a pull request right away**, as a draft while it is not done. A branch is checked through its pull request, never on its own, and a draft gets the same run
3. **Squash merge**, or switch on auto-merge. `main` takes the pull request once `Check` and `Title` have passed, as one commit under its title, and deletes the branch

- The commits inside the branch are yours to shape. Only the title reaches `main`
- A draft cannot be merged. Marking it ready starts nothing, since it changes no code

## The Title

The title is the one line `main` keeps. The next version and the changelog are read out of it, so it follows **[Conventional Commits](https://www.conventionalcommits.org)**: a type, a colon, what changed.

| Title | Version | On the Release Page |
| --- | --- | --- |
| `fix: a secret the machine already has is asked once` | 0.5.0 → 0.5.1 | Bug Fixes |
| `feat: a product may say where the rest of it is` | 0.5.0 → 0.6.0 | Features |
| `feat!: a module declares its hooks by folder` | 0.5.0 → 0.6.0 | ⚠ BREAKING CHANGES |
| `docs:` `refactor:` `test:` `build:` `ci:` `chore:` | none | nothing |

- Write it for somebody building a product. `Title` refuses one that opens on no type, and reads it again whenever it is edited
- Below 1.0.0 a break moves the minor rather than the major: 1.0.0 is a decision, not a count
- Merging shows the title and an empty description. Leave both as they are, with two exceptions typed into the description:
  - `BREAKING CHANGE: …` says on the release page what a product has to change
  - `Release-As: 1.0.0` sets a version that is chosen rather than counted

**Note:** _A title that turns out wrong after the merge is corrected in the description of the merged pull request. The next release run reads this instead of the commit:_

```
BEGIN_COMMIT_OVERRIDE
fix: the corrected line
END_COMMIT_OVERRIDE
```

## Releasing

Nothing is typed and nothing is tagged by hand.

1. **Every merge that releases something** opens or updates the pull request `chore(main): release 0.6.0`. It writes that version's section of **[CHANGELOG.md](../CHANGELOG.md)**
2. **Merging it is the release.** It starts no run of its own, so an admin merges it past the checks: `gh pr merge <number> --squash --admin`. The run on `main` tags `v0.6.0`, checks and builds `oak-linux-amd64` as that version, hangs it on the release page and publishes it

- Merges collect in the release pull request until it is merged. When to release is a decision, not a schedule
- The version lives in `.release-please-manifest.json` alone. The binary answers it on the `runtime:` line of `--version`, and `make build` refuses a binary that answers anything else
- Every other build answers a pre-release of the next patch: `0.6.1-dev`. `make build VERSION=0.7.0` builds any other
- The changelog is never edited by hand

**Note:** _The page stays a draft until the binary hangs on it. A run that fails on the way leaves a draft: re-run its failed jobs._

**Note:** _What each number promises a product is written down once, in the **[README](README.md#1-get-oak)**._

## What CI Runs

| Job | When | Does |
| --- | --- | --- |
| `Title` | a pull request opened, pushed to or edited | Reads the title |
| `Check` | a pull request, a release, on demand | `make check` with the tests under the race detector, and `make vuln` |
| `Release` | a push to `main` | The release pull request, or once that is merged, the tag and the draft page |
| `Publish` | a release | Hangs the binary `Check` built on the page, signed, and publishes it |

**Note:** _A merge into `main` runs `Release` alone: its pull request has passed `Check` already._

## Doing the Work

```
make check                     # the whole gate, as CI runs it
make fmt                       # format the Go and the shell
make run                       # Oak against the example product
make run MODULE=setup          # opens one module directly
make run ARGS=--debug          # ...without touching anything
make inspect                   # loads the example the way a run does
make build                     # bin/oak-linux-amd64, the file a release publishes
make vuln                      # known vulnerabilities in what this imports
make locales                   # the template, and every catalog brought up to it
```

```
sudo pacman -S --needed go gcc make shellcheck shfmt yamllint actionlint zizmor gettext govulncheck gitleaks
```

**Note:** _CI installs the same packages and runs the same commands in an Arch container. There is no second definition of green._

### The Boundary

Oak draws, asks, keeps and runs. It knows nothing about what is being installed.

If a change would put the word `pacman`, `btrfs`, `GNOME` or `LUKS` anywhere in this repository, it belongs in a product. Find the general capability a module is missing and add that instead.

**Note:** _Oak must not know a module by name either. `installer` and `recovery` are folder names in somebody's product, not words in this code._

## Translating

Every sentence Oak shows is translatable, and the English sentence is its own key. Reword one and the old translation is marked fuzzy rather than dropped; delete it and the translation goes with it.

```
make locales   # in the same change as anything added, reworded or deleted on screen
```

**Note:** _`make check` refuses a stale template and a translation that has lost a placeholder._

## Pictures in the Docs

Every image under `docs/` is generated, so none outlives the interface it shows. CI never renders them: they are run by hand and committed with the change.

```
docs/screenshots.sh   # after any visible change to a page
docs/banner.sh        # after the screenshots, the wordmark or the accent changed
```

- They need `chromium`, `imagemagick`, `python-pyte` and `python-yaml`, none of which a build or `make check` needs
- Every run is started with `--debug` against the example, so nothing is built while it is photographed. Which pages are taken is **[screenshots.yaml](screenshots.yaml)**
- Every page comes out the same on every run except `run.png`, which catches the run while it is going

## The Repository

What GitHub holds this repository to is kept in **[.github/settings/](../.github/settings)** rather than clicked. An admin logged in with `gh` applies it:

```
make github
```

| File | Says |
| --- | --- |
| `repository.json` | Squash merges only, under the pull request's title alone; auto-merge allowed; a merged branch is deleted |
| `ruleset.json` | `main` takes nothing but a pull request once `Check` and `Title` have passed; an admin may merge one past them, which the release pull request needs. No force push, no deletion |
| `actions.json` | A workflow's token reads unless it says otherwise, and may open the release pull request |
| `code-scanning.json` | CodeQL's default setup for Go, on pull requests, on `main` and weekly. The workflows are zizmor's, in `make check` |

Run it again after changing one of them. The script is the same in every project released this way; `ruleset.json` names each project's checks, and `code-scanning.json` is optional.
