# Build targets for codex-tui.
# `make build` produces a size-reduced binary with the Go linker's
# `-s -w` flags (strip symbol table and DWARF debug info).

# Output binary; build artifacts under bin/ are git-ignored.
BINARY ?= bin/codex-tui

# Linker flags. Override on the command line to keep debug info:
#   make build LDFLAGS=
LDFLAGS ?= -s -w

# Reproducible path handling: drop local directory info from the binary.
BUILDFLAGS ?= -trimpath

GO ?= go

.PHONY: all build test vet check run tidy clean

# Default target.
all: build

# Compressed release-style build.
build:
	@mkdir -p $(dir $(BINARY))
	$(GO) build $(BUILDFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

# Run the full test suite (as documented in README).
test:
	$(GO) test ./...

# Static analysis (as documented in README).
vet:
	$(GO) vet ./...

# Vet plus tests.
check: vet test

# Run from source without producing a binary.
run:
	$(GO) run .

# Synchronize go.mod/go.sum.
tidy:
	$(GO) mod tidy

# Remove build output.
clean:
	rm -f $(BINARY)
