# Contributing

Oak is one binary and nothing else ships. What is checked lives in the `Makefile`, CI runs the same commands, and a release is a tag on a commit those commands already passed.

## Branches

There is one long-lived branch, `main`. Work happens on a branch off it and comes back through a pull request.

```mermaid
flowchart LR
    M["main"] -->|branch off| F["feature/*"]
    F -->|pull request| C["CI checks it"]
    C -->|squash merge| M2["main<br/><small>one commit per change</small>"]
    M2 -->|tag v1.0.0| R["Release<br/><small>artefacts of that commit</small>"]

    style R fill:#8fbcbb,stroke:#8fbcbb,color:#2e3440
```

- Branch off `main`, name it `feature/<what>`
- Open a pull request. CI checks it
- **Merge with squash.** One pull request is one commit, so `main` stays a straight line and its title is what ends up in the history

**Repository settings this relies on** — Settings → General → Pull Requests:

| Setting | Value |
| --- | --- |
| Allow merge commits | off |
| Allow squash merging | on |
| Allow rebase merging | off |
| Require linear history (branch protection on `main`) | on |

## Releasing

A release is a `v*` tag on `main`. It publishes the artefacts of the commit it points at — nothing is rebuilt for it.

Draft it in the browser: **Releases** → **Draft a new release** → **Choose a tag**, type `v1.0.0`, **Create new tag on publish** → target `main` → **Publish release**.

Or from a terminal:

```
git tag v1.0.0
git push origin v1.0.0
```

Both land in the same place. A pushed tag has no release yet, so CI writes one with generated notes; a release published from the page already has its notes, so CI only hangs `oak-linux-amd64` and its checksum on it once the checks are green.

The version comes out of `git describe`, so the tag is what the binary answers with. Nothing else has to be edited.

**Note:** _The `v` is what CI watches for. A tag without it builds nothing and releases nothing._

**Note:** _Semantic versions. A change to what a product may declare is a minor version; a change that stops an existing product from loading is a major one._

## What CI runs

Once per pull request, once per push to `main`, and once more on a tag.

```mermaid
flowchart TD
    P["pull request · main · tag"] --> C["check<br/><small>make check · race detector</small>"]
    P --> S["security<br/><small>govulncheck · gitleaks</small>"]
    P --> B["build<br/><small>oak-linux-amd64 · checksum</small>"]
    C --> R
    S --> R
    B --> R["release<br/><small>only on a v* tag</small>"]

    style R fill:#8fbcbb,stroke:#8fbcbb,color:#2e3440
```

`build` is the only job that compiles anything, and the release publishes that artefact rather than building again. What is downloaded is the file the checks ran against.

## Doing the work

```
make check                   # everything that has to pass before a commit
make run                     # Oak against the example product
make run MODULE=hello        # opens one module directly
make run ARGS=--debug        # ...without touching anything
make inspect                 # loads the example the way a run does
make locales                 # the template, and every catalog brought up to it
make release                 # bin/oak-linux-amd64, plus its checksum
```

Install the required packages:

```
sudo pacman -S --needed go gcc make shellcheck shfmt staticcheck yamllint actionlint gettext govulncheck gitleaks
```

**Note:** _CI installs the same packages and runs the same commands in an Arch container. There is no second definition of green._

## The boundary

Oak draws, asks, keeps and runs. It knows nothing about what is being installed.

If a change would put the word `pacman`, `btrfs`, `GNOME` or `LUKS` anywhere in this repository, the change belongs in a product rather than here. Find the general capability a module is missing and add that instead.

**Note:** _Oak must not know a module by name either. `installer` and `recovery` are folder names in somebody's product, not words in this code._

## Words on screen

Every sentence Oak shows is translatable and the English sentence is its own key, so writing one is writing the source text and the key at once. Reword it and the old translation is marked fuzzy rather than dropped.

```
make locales   # after adding, rewording or deleting anything on screen
```

**Note:** _`make check` refuses a stale template and a translation that has lost a placeholder._

## Commits

- Imperative mood (`Add`, `Fix`, `Refactor`), one logical change per commit
- Commits are squashed on merge, so the pull request title is what ends up in the history
