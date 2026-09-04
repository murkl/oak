APP     := oak
PKG     := .
BIN_DIR := bin

# The version: the tag this commit carries, or the nearest one with the distance
# and the short SHA after it. `make run` appends "-dev" so it is obvious a binary
# did not come from a build of an actual release.
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_DEV := $(VERSION)-dev

GOOS   ?= linux
GOARCH ?= $(shell go env GOARCH)

# The version lives inside the binary, not in the filename: a stable name keeps
# download links and the builds that follow them working across releases.
BIN := $(BIN_DIR)/$(APP)-$(GOOS)-$(GOARCH)

LDFLAGS_BUILD := -s -w -X main.version=$(VERSION)
LDFLAGS_RUN   := -X main.version=$(VERSION_DEV)
GOFLAGS       := CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH)

# The product this is developed against: the smallest whole thing Oak can drive,
# an oak.yaml with a modules folder beside it. The binary looks next to itself
# and nowhere else, so it is built into that folder rather than run out of a
# temporary one.
#
# MODULE opens one outright, the way `oak hello` does on a machine; without it
# the interface asks which. ARGS is whatever else that run takes — `make run
# ARGS=--debug` for one that touches nothing.
EXAMPLE := example
MODULE  ?=
ARGS    ?=

# The template every catalog here is filled in from, and the catalogs
# themselves. Both are generated: the template out of the Go sources, the
# catalogs out of the template.
POT      := locales/$(APP).pot
CATALOGS := $(wildcard locales/*.po)

.PHONY: all build release example run inspect lint tidy tidy-check test test-race vet staticcheck vuln fmt fmt-check locales locales-check check clean

all: build

build: $(BIN_DIR)
	$(GOFLAGS) go build -trimpath -ldflags="$(LDFLAGS_BUILD)" -o $(BIN) $(PKG)
	cd $(BIN_DIR) && sha256sum $(notdir $(BIN)) > $(notdir $(BIN)).sha256

# What a release holds: one binary, for the one platform an installer runs on.
release:
	$(MAKE) build GOOS=linux GOARCH=amd64

# Straight from source, into the example product.
example:
	go build -ldflags="$(LDFLAGS_RUN)" -o $(EXAMPLE)/$(APP) $(PKG)

run: example
	cd $(EXAMPLE) && ./$(APP) $(MODULE) $(ARGS)

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
		msgfmt --check-format --statistics -o /dev/null "$$po" || exit 1; \
	done

fmt:
	gofmt -s -w .

# gofmt asked as a question rather than made as an edit, so a branch that was
# never formatted fails here instead of arriving later as a diff nobody wrote.
fmt-check:
	@unformatted="$$(gofmt -s -l .)"; \
	[ -z "$$unformatted" ] || { echo "not gofmt'd:" >&2; echo "$$unformatted" >&2; exit 1; }

# The scripts of the example product are sourced, never executed, so they are
# checked the way Oak runs them: as bash, with lib.sh already in scope.
lint:
	shellcheck -x $(wildcard $(EXAMPLE)/modules/*/lib.sh) $(shell find $(EXAMPLE) -name 'task.sh')
	shfmt -d -i 4 $(shell find $(EXAMPLE) -name '*.sh')
	yamllint .
	actionlint

# What has to pass before anything is committed.
check: fmt-check tidy-check vet staticcheck locales-check lint test build inspect

# There is deliberately no install target: the binary looks for its oak.yaml
# beside itself, so a copy on $$PATH with nothing next to it can only say there
# is nothing to run. What ships is the modules with the binary beside them.

clean:
	rm -rf $(BIN_DIR) $(EXAMPLE)/$(APP)

$(BIN_DIR):
	mkdir -p $@
