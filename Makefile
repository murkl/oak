# Recipes are bash and stop at the first failing command, in loops and pipes
# too.
SHELL       := bash
.SHELLFLAGS := -eu -o pipefail -c

APP     := oak
PKG     := .
BIN_DIR := bin

# The last release, as release-please keeps it.
RELEASED := $(shell sed -n 's/.*"\.":[[:space:]]*"\([^"]*\)".*/\1/p' .release-please-manifest.json)

# What `oak --version` answers on its `runtime:` line: the release its run hands
# in as `VERSION=`, otherwise a pre-release of the next patch.
VERSION := $(shell echo '$(RELEASED)' | awk -F. '{ print $$1 "." $$2 "." $$3 + 1 "-dev" }')

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

# Bash: the example's, sourced with its dialect in the .shellcheckrc beside it,
# and the helpers in docs/.
SCRIPTS := $(shell find $(EXAMPLE) -name '*.sh') $(wildcard docs/*.sh)

# POSIX sh, executed rather than sourced.
POSIX_SCRIPTS := .github/settings.sh

# The template, generated out of the Go sources, and the catalogs out of it.
POT      := locales/$(APP).pot
CATALOGS := $(wildcard locales/*.po)

.PHONY: all build example run inspect lint tidy tidy-check test vet staticcheck vuln secrets-check fmt fmt-check locales locales-check check github clean

all: build

# What ships, built on the way through `check`, so the file checked is the file
# published. Read back, since -X sets nothing once the variable is renamed.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN) $(PKG)
	@said="$$(./$(BIN) --version | sed -n 's/^runtime: //p')"; [ "$$said" = '$(VERSION)' ] \
		|| { echo "$(BIN) answers '$$said', not $(VERSION)" >&2; exit 1; }

# Straight from source, into the example, for this machine.
example:
	go build -ldflags="-X main.version=$(VERSION)" -o $(EXAMPLE)/$(APP) $(PKG)

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

# Dead code and the mistakes vet does not look for, at the release go.mod
# names: a newer Go needs a newer reader of its export data.
staticcheck:
	go tool staticcheck ./...

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

# zizmor runs offline, so a finding is about a change here.
lint:
	shellcheck -x $(SCRIPTS)
	shellcheck -s sh -S style $(POSIX_SCRIPTS)
	yamllint .
	actionlint
	zizmor --offline --persona auditor .github

# What has to pass before anything is committed.
check: fmt-check tidy-check vet staticcheck secrets-check locales-check lint test build inspect

# The repository's settings on GitHub, out of .github/settings/. Run by hand as
# an admin: a workflow may not change the rules it is held to.
github:
	.github/settings.sh

# No install target: the binary looks for its oak.yaml beside itself.

clean:
	rm -rf $(BIN_DIR) $(EXAMPLE)/$(APP)
