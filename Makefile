# Recipes are bash, and they stop at the first command that fails — including
# inside a loop and inside a pipe, where a check otherwise passes quietly.
SHELL       := bash
.SHELLFLAGS := -eu -o pipefail -c

APP     := oak
PKG     := .
BIN_DIR := bin

# The version: the tag this commit carries, or the nearest one with the distance
# and the short SHA after it. `make run` appends "-dev" so it is obvious a binary
# did not come from a build of an actual release.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

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

.PHONY: all build example run inspect lint tidy tidy-check test test-race vet staticcheck vuln fmt fmt-check locales locales-check version-check check clean

all: build

# What ships, and what `make check` builds on the way through: the release
# artefact itself, so the file the checks ran against is the file published.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN) $(PKG)
	cd $(BIN_DIR) && sha256sum $(notdir $(BIN)) > $(notdir $(BIN)).sha256

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

# What a tag is allowed to release. The version is `git describe` and there is
# no second place to keep in step with it, so what can still go wrong is a tag
# whose binary does not answer to it: a tag moved after the fact, a clone too
# shallow to describe one, a tree with edits in it. Any of those would publish
# a version nothing inside the file agrees with.
#
#   make version-check TAG=v1.0.0                     against what build wrote
#   make version-check TAG=v1.0.0 BIN=dist/oak-...    against what CI will ship
version-check:
	@[[ "$(TAG)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$$ ]] \
		|| { echo "not a release tag: '$(TAG)' — a release is vMAJOR.MINOR.PATCH" >&2; exit 1; }
	@said="$$(./$(BIN) --version)"; \
	[ "$$said" = "$(APP) $(TAG)" ] \
		|| { echo "$(BIN) answers '$$said' — the tag says '$(TAG)'" >&2; exit 1; }
	@echo "$(BIN) is $(TAG)"

# What has to pass before anything is committed.
check: fmt-check tidy-check vet staticcheck locales-check lint test build inspect

# There is deliberately no install target: the binary looks for its oak.yaml
# beside itself, so a copy on $$PATH with nothing next to it can only say there
# is nothing to run. What ships is the modules with the binary beside them.

clean:
	rm -rf $(BIN_DIR) $(EXAMPLE)/$(APP)
