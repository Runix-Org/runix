# Show available development commands.
default:
    @just --list

# Prepare the pinned development tools and Go dependencies after cloning.
setup:
    mise trust mise.toml
    mise exec -- go mod download

# Format Go sources in place with pinned gofumpt.
fmt:
    GOFLAGS="-tags=devtools" golangci-lint fmt ./...

# Reject non-gofumpt formatting without changing sources; print a diff.
fmt-check:
    GOFLAGS="-tags=devtools" golangci-lint fmt --diff ./...

# Run Go's static checks.
vet:
    GOFLAGS="-tags=devtools" go vet ./...

# Run pinned linters and format checks without changing source or module files.
lint:
    GOFLAGS="-tags=devtools" golangci-lint run ./...

# Apply supported linter fixes; source files may change.
lint-fix:
    GOFLAGS="-tags=devtools" golangci-lint run --fix ./...

# Run all project tests.
test:
    GOFLAGS="-tags=devtools" go test ./...

# Check module files without rewriting go.mod or go.sum.
mod-check:
    go mod tidy -diff

# Build a local binary without installing it.
build:
    GOFLAGS="-tags=devtools" go build .

# Build a local release binary without debugger symbols or DWARF information.
build-release:
    go build -tags='' -ldflags="-s -w" .

# Build and replace the user-local executable and desktop entry.
install: build-release
    #!/bin/sh
    set -eu
    install -Dm755 runix "$HOME/.config/bin/runix"
    install -Dm644 data/runix.desktop "$HOME/.local/share/applications/runix.desktop"
    awk '
        /^Exec=/ {
            path = ENVIRON["HOME"] "/.config/bin/runix"
            printf "Exec=\""
            for (i = 1; i <= length(path); i++) {
                c = substr(path, i, 1)
                if (c == "\\") printf "\\\\\\\\"
                else if (c == "\"" || c == "$" || c == "`") printf "\\\\%s", c
                else if (c == "%") printf "%%%%"
                else printf "%s", c
            }
            print "\""
            next
        }
        { print }
    ' data/runix.desktop > "$HOME/.local/share/applications/runix.desktop"

# Run the CLI, forwarding arguments.
run *args:
    GOFLAGS="-tags=devtools" go run . {{args}}

# Run the checks required before committing; never rewrite source files.
precommit: lint test build
