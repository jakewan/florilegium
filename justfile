# Build the binary
build:
    @mkdir -p bin
    go build -o bin/florilegium ./cmd/florilegium

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

# Clean build artifacts
clean:
    rm -rf bin/
    rm -f coverage.out

# Install git hooks via lefthook
hooks:
    lefthook install
