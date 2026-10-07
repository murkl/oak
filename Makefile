# Recipes are bash and stop at the first failing command, in loops and pipes
# too.
SHELL       := bash
.SHELLFLAGS := -eu -o pipefail -c

APP     := oak
PKG     := .
BIN_DIR := bin

# The version: the tag on this commit, or the last one before it, without its
# `v`. It is what `oak --version` answers on its `runtime:` line; `make run`
# appends "-dev".
VERSION := $(or $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'),dev)

# What a release is called. CI hands in the tag its release run wrote.
TAG ?= v$(VERSION)

# One binary for the one platform an installer runs on. A stable name keeps
# download links working; the version lives inside the file.
GOOS   ?= linux
GOARCH ?= amd64
BIN    := $(BIN_DIR)/$(APP)-$(GOOS)-$(GOARCH)

# The product this is developed against. The binary looks beside itself, so it
# is built into that folder. MODULE opens one outright, ARGS is the rest:
# `make run ARGS=--debug`.
EXAMPLE := example
MODULE  ?=
ARGS    ?=

# The example's shell: sourced, never executed, so its dialect is in the
# .shellcheckrc beside it.
SCRIPTS := $(shell find $(EXAMPLE) -name '*.sh')

# POSIX sh, executed rather than sourced.
POSIX_SCRIPTS := .github/settings.sh

# The template, generated out of the Go sources, and the catalogs out of it.
POT      := locales/$(APP).pot
CATALOGS := $(wildcard locales/*.po)

# The pictures in docs/, generated so they cannot drift: the screenshots by
# driving the example with --debug, the banner out of two of them. Which pages
# is docs/screenshots.yaml. They need chromium, imagemagick, python-pyte and
# python-yaml, so they stay out of `check`.
BANNER_CARDS   := docs/screenshots/report.png docs/screenshots/run.png
BANNER_TAGLINE := Build your own Arch Linux distribution. The installer is already written.
BANNER_CELL    := 17

.PHONY: all build example run inspect lint tidy tidy-check test vet staticcheck vuln secrets-check fmt fmt-check locales locales-check tag-check version-check check github screenshots banner docs clean

all: build

# What ships, built on the way through `check`, so the file checked is the file
# published.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN) $(PKG)

# Straight from source, into the example, for this machine.
example:
	go build -ldflags="-X main.version=$(VERSION)-dev" -o $(EXAMPLE)/$(APP) $(PKG)

run: example
	cd $(EXAMPLE) && ./$(APP) $(if $(MODULE),--module=$(MODULE)) $(ARGS)

# The example loaded as a run loads it: the one check that reads yaml, so a
# change to what a product may declare fails on a real product first.
inspect: example
	@cd $(EXAMPLE) && ./$(APP) --inspect

tidy:
	go mod tidy

# What `tidy` would change, as an error.
tidy-check:
	go mod tidy -diff

# Under the race detector, which needs cgo and gcc: the runner and the
# interface share state across goroutines, and a race shows nowhere else.
test:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

# Dead code and the mistakes vet does not look for.
staticcheck:
	staticcheck ./...

# Known vulnerabilities in what this imports. It asks a server, so it stays out
# of `check`.
vuln:
	govulncheck ./...

# Other projects' builds download and run this binary.
secrets-check:
	gitleaks dir . --redact --no-banner

# The template out of every T("…"), and each catalog brought up to it: a changed
# text turns fuzzy, a removed one is dropped.
locales:
	go run ./tools/potgen > $(POT)
	@for po in $(CATALOGS); do \
		msgmerge --quiet --update --backup=none --no-wrap "$$po" $(POT); \
		msgattrib --no-obsolete --no-wrap -o "$$po" "$$po"; \
	done

# A reworded text needs `make locales`, or no translator sees it. A
# translation that drops a placeholder breaks where it is printed.
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
	shfmt -w -ln posix -i 4 $(POSIX_SCRIPTS)

# Formatting asked as a question rather than made as an edit.
fmt-check:
	@unformatted="$$(gofmt -s -l .)"; \
	[ -z "$$unformatted" ] || { echo "not gofmt'd:" >&2; echo "$$unformatted" >&2; exit 1; }
	shfmt -d $(SCRIPTS)
	shfmt -d -ln posix -i 4 $(POSIX_SCRIPTS)

# zizmor runs offline, so a finding is about a change here. What it leaves
# alone is .github/zizmor.yml.
lint:
	shellcheck -x $(SCRIPTS)
	shellcheck -s sh -S style $(POSIX_SCRIPTS)
	yamllint .
	actionlint
	zizmor --offline --persona auditor .github

# A release tag and nothing else, so a tag and its binary are held to one rule.
tag-check:
	@[[ "$(TAG)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$$ ]] \
		|| { echo "not a release tag: '$(TAG)' — a release is vMAJOR.MINOR.PATCH" >&2; exit 1; }

# A tag may only release a binary that answers to it: a moved tag or a clone too
# shallow to describe one would publish a version the file disagrees with.
#
#   make version-check                                against what build wrote
#   make version-check TAG=v0.1.0 BIN=dist/oak-...    a published tag against what ships under it
version-check: tag-check
	@said="$$(./$(BIN) --version | sed -n 's/^runtime: //p')"; \
	[ "$$said" = "$(TAG:v%=%)" ] \
		|| { echo "$(BIN) answers '$$said' — the tag says '$(TAG)'" >&2; exit 1; }
	@echo "$(BIN) is $(TAG)"

# What has to pass before anything is committed.
check: fmt-check tidy-check vet staticcheck secrets-check locales-check lint test build inspect

# The repository's settings on GitHub, out of .github/settings/. Run by hand as
# an admin: a workflow may not change the rules it is held to.
github:
	.github/settings.sh

# No install target: the binary looks for its oak.yaml beside itself.

screenshots: example
	python3 docs/screenshots.py --product $(EXAMPLE)

banner:
	python3 docs/banner.py \
		--product $(EXAMPLE)/oak.yaml \
		--logo docs/logo.svg \
		$(foreach c,$(BANNER_CARDS),--card $(c)) \
		--tagline "$(BANNER_TAGLINE)" \
		--cell $(BANNER_CELL)

# The banner collages the screenshots, so it comes after them.
docs:
	$(MAKE) screenshots
	$(MAKE) banner

clean:
	rm -rf $(BIN_DIR) $(EXAMPLE)/$(APP)
