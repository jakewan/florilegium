# Version stamped into the binary. No --always: on a tag-less repo git describe
# would otherwise succeed with a bare commit SHA and the `|| echo dev` fallback
# would never fire. Without it, describe fails cleanly when no tag exists and the
# fallback yields "dev". So: tag-less -> dev; clean tagged checkout -> v0.1.0;
# later commits -> v0.1.0-N-gSHA; dirty tree -> a -dirty suffix.
version := `git describe --tags --dirty 2>/dev/null || echo dev`

# The -ldflags below injects internal/server.serverVersion — the SAME symbol
# .goreleaser.yaml injects at release time. A rename of that symbol must be
# changed in BOTH places, or one path silently reverts to "dev".

# Build the binary
build:
    @mkdir -p bin
    go build -ldflags "-X github.com/jakewan/florilegium/internal/server.serverVersion={{version}}" -o bin/florilegium ./cmd/florilegium

# Install the binary to ~/.local/bin (atomic cp+mv — survives "text file busy")
install: build
    @mkdir -p ~/.local/bin
    # cp+mv, not cp alone: mv swaps the directory entry atomically, so the install
    # succeeds even while an older copy is running (cp alone fails with "text file
    # busy" on Linux when overwriting a running binary).
    cp bin/florilegium ~/.local/bin/florilegium.tmp && mv ~/.local/bin/florilegium.tmp ~/.local/bin/florilegium
    @echo "Installed to ~/.local/bin/florilegium"
    @echo "(ensure ~/.local/bin is in your PATH)"

# Run all tests
test:
    go test ./...

# Run tests with coverage
test-coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# Lint with golangci-lint
lint:
    golangci-lint run ./...

# Format code
fmt:
    gofmt -w .

# Tidy module dependencies
tidy:
    go mod tidy

# Verify module dependencies
verify:
    go mod verify

# Preview the changelog git-cliff would generate from unreleased commits, to
# review/polish before a release. See the ritual in .claude/rules/pr-conventions.md.

# Preview the unreleased changelog section
changelog:
    git cliff --unreleased

# --snapshot skips cosign signing and never runs provenance (CI-only,
# OIDC-bound), so this proves the build, not the hardening.

# Validate release config and dry-run a snapshot build (mirrors the CI gate)
release-check:
    goreleaser check
    goreleaser build --snapshot --single-target --clean

# Clean build artifacts
clean:
    rm -rf bin/ dist/
    rm -f coverage.out

# Install git hooks via lefthook
hooks:
    lefthook install
