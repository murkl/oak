# Changelog

What each release changed, newest first. Written by the run that publishes it, out of what landed on `main` since the release before — never by hand. The version is what `oak --version` answers and what a product pins itself to; what moves which number is the promise in the **[README](docs/README.md#1-get-oak)**.

## [0.20.1](https://github.com/murkl/oak/compare/v0.20.0...v0.20.1) (2026-10-07)


### Bug Fixes

* each password before the work stands under the menu, not inside the one before it ([#53](https://github.com/murkl/oak/issues/53)) ([23fe70d](https://github.com/murkl/oak/commit/23fe70df398dc0dba26d14f690f35c168dcb9755))

## [0.20.0](https://github.com/murkl/oak/compare/v0.19.0...v0.20.0) (2026-10-07)


### Features

* headings in a colour of their own, and every colour on the Linux console in the slot of its hue ([69c0ff1](https://github.com/murkl/oak/commit/69c0ff12df8345ba46e866f6e47a149ede30d1fd))
* the menu names itself in the breadcrumb, and the pages it opens stand behind it ([ffd2c45](https://github.com/murkl/oak/commit/ffd2c457e4342e79ddb102da8b7e6d0340d26fd2))


### Bug Fixes

* walking up a list scrolls it only once the cursor reaches the top ([4e87bd6](https://github.com/murkl/oak/commit/4e87bd6d55587c41b3dda2f70b28a4c32fe173bc))

## [0.19.0](https://github.com/murkl/oak/compare/v0.18.1...v0.19.0) (2026-10-03)


### ⚠ BREAKING CHANGES

* no fades or effects but the wordmark's sweep, the product's version under it, and --version for both ([#48](https://github.com/murkl/oak/issues/48))

### Features

* no fades or effects but the wordmark's sweep, the product's version under it, and --version for both ([#48](https://github.com/murkl/oak/issues/48)) ([48316ea](https://github.com/murkl/oak/commit/48316ea29bb467809c8988159d3db8f9bd74b3cf))
* the question of which module to open reads What would you like to start? ([#46](https://github.com/murkl/oak/issues/46)) ([2aca670](https://github.com/murkl/oak/commit/2aca6705166957082fc13577d9d9e31a77181bbd))


### Bug Fixes

* the last row of a list stays on the last line of its window ([#45](https://github.com/murkl/oak/issues/45)) ([a17bb5b](https://github.com/murkl/oak/commit/a17bb5ba109361eb5e7a880cf5a6867b5c9f0438))

## [0.18.1](https://github.com/murkl/oak/compare/v0.18.0...v0.18.1) (2026-10-03)


### Bug Fixes

* the last page has no breadcrumb ([#43](https://github.com/murkl/oak/issues/43)) ([221d128](https://github.com/murkl/oak/commit/221d12850eeaab8f579330e69fff217d2926880d))

## [0.18.0](https://github.com/murkl/oak/compare/v0.17.1...v0.18.0) (2026-10-02)


### ⚠ BREAKING CHANGES

* every question names its type, and an action asks a list of variables under the same rules as a module ([#41](https://github.com/murkl/oak/issues/41))
* the words of a module's pages under text:, the last page after every password in the module's own words, and every confirm opening on No ([#40](https://github.com/murkl/oak/issues/40))

### Features

* every question names its type, and an action asks a list of variables under the same rules as a module ([#41](https://github.com/murkl/oak/issues/41)) ([40c3ca2](https://github.com/murkl/oak/commit/40c3ca23747258e7841b1b86992c51b2db80f04b))
* the words of a module's pages under text:, the last page after every password in the module's own words, and every confirm opening on No ([#40](https://github.com/murkl/oak/issues/40)) ([eafd411](https://github.com/murkl/oak/commit/eafd411b875547f02457b53804398bbf7be7aadd))

## [0.17.1](https://github.com/murkl/oak/compare/v0.17.0...v0.17.1) (2026-10-02)


### Bug Fixes

* no script inherits a descriptor the interface opened ([57cc18f](https://github.com/murkl/oak/commit/57cc18f58f995c40a81e21693d79fc5b187a2278))

## [0.17.0](https://github.com/murkl/oak/compare/v0.16.0...v0.17.0) (2026-10-02)


### ⚠ BREAKING CHANGES

* test results on a row of their own, shell named as name() or ./file.sh, keys named after what they hold, and a last warning that says it cannot be undone ([#36](https://github.com/murkl/oak/issues/36))

### Features

* test results on a row of their own, shell named as name() or ./file.sh, keys named after what they hold, and a last warning that says it cannot be undone ([#36](https://github.com/murkl/oak/issues/36)) ([75b1313](https://github.com/murkl/oak/commit/75b131348de9dab9f177352cf44d92bb90f94eef))

## [0.16.0](https://github.com/murkl/oak/compare/v0.15.0...v0.16.0) (2026-10-02)


### ⚠ BREAKING CHANGES

* `rules: menu` is `rules: settings`, and its rows stand on the settings page. A variable a task names under `asks:` declares `type: deferred`, and a yes or no in the middle of a run is the task's `confirm:`. The last page before the work opens on No.

### Features

* actions on the settings page, a yes or no before the work, and deferred questions declared as such ([#34](https://github.com/murkl/oak/issues/34)) ([78c1c81](https://github.com/murkl/oak/commit/78c1c812efa79b5f66531cd2733597982efd40e2))

## [0.15.0](https://github.com/murkl/oak/compare/v0.14.0...v0.15.0) (2026-10-01)


### ⚠ BREAKING CHANGES

* `offered:`, `requires:`, `menu:`, `leave:`, `failure:` and `success:` in `module.yaml` move under `rules:` as `offer-if`, `start-if`, `menu`, `on-leave`, `on-failure` and `on-success`; an action's `requires:` and `fallback:` move under its `rules:` as `offer-if` and `on-failure`. `start:` is `start-title:`. A preset's `options:` are refused: every option is a preset of its own, and the page's title and description are gone. `module.sh` and a product's `actions/` are refused: shared shell goes into `oak.sh`, and an action into the module that names it.

### Features

* rules for actions, one shared shell, and one page of starting points ([#32](https://github.com/murkl/oak/issues/32)) ([ca9ad76](https://github.com/murkl/oak/commit/ca9ad76d5eeca0ca444a19763e98e6542da803ee))

## [0.14.0](https://github.com/murkl/oak/compare/v0.13.0...v0.14.0) (2026-10-01)


### ⚠ BREAKING CHANGES

* an action's `script:`, `variables:`, `confirm:` and `default:` are refused, and so are a task's `script:`, `test:`, `default:`, `shows:`, `quits:` and `tty:`, and a preset option's `asks:` and `apply:`. A task does its work in `task.sh` and is tested by `test.sh`; an action does its work in `action.sh`, says what a no means under `fail:` - required under `offered:` and `requires:` - and has one page at most: `variable:`, `report:` or `tty:`. A second question is a second action named as the `fallback:`. What was offered at the end of a run as a task is an action under the new `success:`. A fetched starting point names the action that fetches it under `action:`.

### Features

* one-page actions with fail and fallback, success rows and actions shared beside oak.yaml ([#29](https://github.com/murkl/oak/issues/29)) ([f75c443](https://github.com/murkl/oak/commit/f75c4430b5ba70f6bb0c37a26c3a814ac824763c))

## [0.13.0](https://github.com/murkl/oak/compare/v0.12.0...v0.13.0) (2026-09-30)


### ⚠ BREAKING CHANGES

* a module's hooks/, options/, network:, console:, its confirm: and action: are refused. Everything a module does outside its work is an action under actions/, named in module.yaml where it runs: offered: in place of the shell requires: was, requires: for what the work waits for (a hook @preflight or @online), menu:, leave: (@restart, @shutdown) and failure:. A wireless network is an action with pages, named as the fallback of the one that checks the internet. action: is start:. console: and the module's confirm: go: the runtime says the last page and the way out itself.

### Features

* actions, named where they run, in place of options ([#28](https://github.com/murkl/oak/issues/28)) ([4f44cb3](https://github.com/murkl/oak/commit/4f44cb3265e36e6296b9753cdb9775ab74b1b827))
* the modules offered by their names alone ([#25](https://github.com/murkl/oak/issues/25)) ([49de415](https://github.com/murkl/oak/commit/49de415516fa2f828c8a8c9615b5efd93f491300))

## [0.12.0](https://github.com/murkl/oak/compare/v0.11.0...v0.12.0) (2026-09-27)


### ⚠ BREAKING CHANGES

* the network hooks no longer switch anything on by being there. A module that needs the internet on the way in declares `network: {internet: required}`, one that joins a wireless network `network: {wlan: true}`.

### Features

* the network declared by the module, waited for where it is required and joined from the settings ([#22](https://github.com/murkl/oak/issues/22)) ([05cee05](https://github.com/murkl/oak/commit/05cee05fbade6e96e6c80426634a82ee58f931e3))

## [0.11.0](https://github.com/murkl/oak/compare/v0.10.0...v0.11.0) (2026-09-25)


### Features

* the module chosen under the wordmark, and a module's own word for starting it ([#20](https://github.com/murkl/oak/issues/20)) ([00553fa](https://github.com/murkl/oak/commit/00553facd2b2061cfd9f29f552d44175fe425ada))

## [0.10.0](https://github.com/murkl/oak/compare/v0.9.0...v0.10.0) (2026-09-24)


### Features

* a welcome page of its own, a status in the header, a network on the menu and a password checked where it is typed ([#18](https://github.com/murkl/oak/issues/18)) ([772bcbf](https://github.com/murkl/oak/commit/772bcbf290319a6c5ac37e593beea5fd6cdbb3c1))

## [0.9.0](https://github.com/murkl/oak/compare/v0.8.0...v0.9.0) (2026-09-24)


### ⚠ BREAKING CHANGES

* `url` is no longer a key of oak.yaml. The welcome page was its only reader. Delete the line.

### Features

* name the language and run as a kiosk from the command line, and greet in one sentence ([#16](https://github.com/murkl/oak/issues/16)) ([b1c730b](https://github.com/murkl/oak/commit/b1c730b9d20c58aeab881ac20bd13fba06c05b20))

## [0.8.0](https://github.com/murkl/oak/compare/v0.7.0...v0.8.0) (2026-09-24)


### Features

* let a task be optional so its failure is listed instead of stopping the run ([ac73a4a](https://github.com/murkl/oak/commit/ac73a4a394679db9795bbdd98601b0bfd1de4a83))

## [0.7.0](https://github.com/murkl/oak/compare/v0.6.0...v0.7.0) (2026-09-24)


### Features

* feature/list answers and progress ([#6](https://github.com/murkl/oak/issues/6)) ([271e0fa](https://github.com/murkl/oak/commit/271e0fa1114fb2e0bfa21f1c2bf7afe11ec8e1f8))

## [0.6.0](https://github.com/murkl/oak/compare/v0.5.0...v0.6.0) (2026-09-23)


### ⚠ BREAKING CHANGES

* add a shared product shell, Oak-side simulation, a real terminal handover and --glyphs

### Features

* add a shared product shell, Oak-side simulation, a real terminal handover and --glyphs ([3e2eb98](https://github.com/murkl/oak/commit/3e2eb98cec1c24bce1b6b406db07ff6af4808f14))

## [0.5.0](https://github.com/murkl/oak/compare/v0.4.1...v0.5.0) (2026-09-19)


### Features

* ask a password the machine already has once rather than twice ([45a24b2](https://github.com/murkl/oak/commit/45a24b28bc14e4edb59f83970e2219ade679e0c7))


### Bug Fixes

* refuse a confirm or report naming a variable nothing declares ([51bf6b6](https://github.com/murkl/oak/commit/51bf6b67e63008cae6a6e6ed265d2dcdabe46052))
* refuse a translation naming other placeholders than its source ([ab6b80a](https://github.com/murkl/oak/commit/ab6b80aec055ba09ab6b2c6b67ad9b8f991b9c58))

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
