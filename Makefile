# Recipes are bash, and they stop at the first command that fails — including
# inside a loop and inside a pipe, where a check otherwise passes quietly.
SHELL       := bash
.SHELLFLAGS := -eu -o pipefail -c

APP     := oak
PKG     := .
BIN_DIR := bin

# The version: the release this commit belongs to — the tag on it, or the last
# one before it. It is the whole of what `oak --version` answers, because a
# product pins the Oak it was built against by that number and nothing beside it
# would survive being read back. `make run` appends "-dev", so a binary that did
# not come from a build of an actual release says so.
#
# The tag's leading `v` is dropped here: it belongs to the tag and to nothing
# else.
VERSION := $(or $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'),dev)

# What a release is called: `v` + the version above. CI hands in the tag its
# release run actually wrote, so what is checked is that tag rather than one
# worked out a second time.
TAG ?= v$(VERSION)

# One binary, for the one platform an installer runs on. Named after neither the
# version nor the host: a stable name keeps download links and the builds that
# follow them working across releases, and the version lives inside the file.
GOOS   ?= linux
GOARCH ?= amd64
BIN    := $(BIN_DIR)/$(APP)-$(GOOS)-$(GOARCH)

# The product this is developed against: the smallest whole thing Oak can drive,
# an oak.yaml with a modules folder beside it. The binary looks next to itself
# and nowhere else, so it is built into that folder rather than run out of a
# temporary one.
#
# MODULE opens one outright, the way `oak --module=setup` does on a machine;
# without it the interface asks which. ARGS is whatever else that run takes —
# `make run ARGS=--debug` for one that touches nothing.
EXAMPLE := example
MODULE  ?=
ARGS    ?=

# The shell the example is made of. Oak sources it and never executes it, so
# none of it carries a shebang: the dialect is in the .shellcheckrc beside it
# and the indent is in .editorconfig, which is where shfmt reads it from.
SCRIPTS := $(shell find $(EXAMPLE) -name '*.sh')

# The template every catalog here is filled in from, and the catalogs
# themselves. Both are generated: the template out of the Go sources, the
# catalogs out of the template.
POT      := locales/$(APP).pot
CATALOGS := $(wildcard locales/*.po)

# The pictures in docs/. Both are generated: the screenshots by driving the
# example on a pty and photographing what it drew, the banner by collaging two
# of them under the wordmark read out of the product's own oak.yaml. A change
# to the interface is one command away from being what the README shows.
#
# Which pages are taken is docs/screenshots.yaml, and every run is started with
# --debug, so nothing here is written to.
#
# They need chromium, imagemagick, python-pyte and python-yaml, which a build
# does not, so they stay out of `check` and are run by hand.
BANNER_CARDS   := docs/screenshots/report.png docs/screenshots/run.png
BANNER_TAGLINE := You write the YAML and the shell. Oak is the program around it.
BANNER_CELL    := 17

.PHONY: all build example run inspect lint tidy tidy-check test test-race vet staticcheck vuln secrets-check fmt fmt-check locales locales-check tag-check version-check check screenshots banner docs clean

all: build

# What ships, and what `make check` builds on the way through: the release
# artefact itself, so the file the checks ran against is the file published.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN) $(PKG)

# Straight from source, into the example product, for this machine.
example:
	go build -ldflags="-X main.version=$(VERSION)-dev" -o $(EXAMPLE)/$(APP) $(PKG)

run: example
	cd $(EXAMPLE) && ./$(APP) $(if $(MODULE),--module=$(MODULE)) $(ARGS)

# The example loaded the way a run loads it: every task ordered, every condition
# resolved, every question checked against the tasks that read it. It is the one
# check here that reads yaml rather than Go, so a change to what a product may
# declare fails on a real product before it reaches anybody else's.
inspect: example
	@cd $(EXAMPLE) && ./$(APP) --inspect

tidy:
	go mod tidy

# What `tidy` would change, said as an error instead of made as a change.
tidy-check:
	go mod tidy -diff

test:
	go test ./...

# The same tests under the race detector, which is built on cgo — asking for it
# explicitly turns a silently skipped check into a missing gcc. Not in `check`:
# it doubles a run that already takes a while. CI runs it on every push.
test-race:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

# Dead code, values that go nowhere, the mistakes vet does not look for.
staticcheck:
	staticcheck ./...

# Known vulnerabilities in what this imports. It asks a server, so it stays out
# of `check`. CI runs it on every push.
vuln:
	govulncheck ./...

# This binary is downloaded and run by other projects' builds, so a credential
# that reached the repository would travel with it. It reads the tree and
# nothing else, which is why it rides along in `check` and `vuln` does not.
secrets-check:
	gitleaks dir . --redact --no-banner

# Reads every T("…") out of the sources and writes the template, then brings
# each catalog up to it. msgmerge keeps every translation whose source text is
# unchanged and marks the rest fuzzy rather than dropping it — a reworded
# sentence is a translation to look at again, not one to write from scratch.
locales:
	go run ./tools/potgen > $(POT)
	@for po in $(CATALOGS); do msgmerge --quiet --update --backup=none --no-wrap "$$po" $(POT); done

# A word added or reworded without `make locales` being run is a word no
# translator will ever be shown. And a translation that drops a placeholder is a
# message that breaks where it is printed rather than where it was written,
# which is the one thing about a catalog that cannot wait for somebody to
# notice.
locales-check:
	@go run ./tools/potgen | diff -u $(POT) - \
		|| { echo "$(POT) is out of date — run 'make locales'" >&2; exit 1; }
	@for po in $(CATALOGS); do \
		printf '%s: ' "$$po"; \
		msgfmt --check-format --statistics -o /dev/null "$$po"; \
	done

fmt:
	gofmt -s -w .
	shfmt -w $(SCRIPTS)

# The same, asked as a question rather than made as an edit, so a branch that
# was never formatted fails here instead of arriving later as a diff nobody
# wrote.
fmt-check:
	@unformatted="$$(gofmt -s -l .)"; \
	[ -z "$$unformatted" ] || { echo "not gofmt'd:" >&2; echo "$$unformatted" >&2; exit 1; }
	shfmt -d $(SCRIPTS)

lint:
	shellcheck -x $(SCRIPTS)
	yamllint .
	actionlint

# A release tag and nothing else. On its own so that the tag a run wrote and the
# binary published under it are held to the same rule.
tag-check:
	@[[ "$(TAG)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$$ ]] \
		|| { echo "not a release tag: '$(TAG)' — a release is vMAJOR.MINOR.PATCH" >&2; exit 1; }

# What a tag is allowed to release. The version is the tag `git describe` finds
# and there is no second place to keep in step with it, so what can still go
# wrong is a tag whose binary does not answer to it: a tag moved after the fact,
# or a clone too shallow to describe one. Either would publish a version nothing
# inside the file agrees with.
#
#   make version-check                                against what build wrote
#   make version-check TAG=v0.1.0 BIN=dist/oak-...    a published tag against what ships under it
version-check: tag-check
	@said="$$(./$(BIN) --version)"; \
	[ "$$said" = "$(TAG:v%=%)" ] \
		|| { echo "$(BIN) answers '$$said' — the tag says '$(TAG)'" >&2; exit 1; }
	@echo "$(BIN) is $(TAG)"

# What has to pass before anything is committed.
check: fmt-check tidy-check vet staticcheck secrets-check locales-check lint test build inspect

# There is deliberately no install target: the binary looks for its oak.yaml
# beside itself, so a copy on $$PATH with nothing next to it can only say there
# is nothing to run. What ships is the modules with the binary beside them.

screenshots: example
	python3 docs/screenshots.py --product $(EXAMPLE)

banner:
	python3 docs/banner.py \
		--product $(EXAMPLE)/oak.yaml \
		--logo docs/logo.svg \
		$(foreach c,$(BANNER_CARDS),--card $(c)) \
		--tagline "$(BANNER_TAGLINE)" \
		--cell $(BANNER_CELL)

# The banner collages the screenshots, so it is drawn after them and never
# beside them, whatever -j says.
docs:
	$(MAKE) screenshots
	$(MAKE) banner

clean:
	rm -rf $(BIN_DIR) $(EXAMPLE)/$(APP)
