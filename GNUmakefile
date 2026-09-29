default: testacc

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

# Run unit tests
.PHONY: test
test:
	go test ./... -v $(TESTARGS) -timeout 120m

# Run every native fuzz target for FUZZTIME each (`make test` only replays
# the seed corpus).
FUZZTIME ?= 10s
.PHONY: fuzz
fuzz:
	@set -e; pkgs=$$(go list ./internal/...); for pkg in $$pkgs; do \
		list=$$(go test -list '^Fuzz' $$pkg); \
		for target in $$(printf '%s\n' "$$list" | grep '^Fuzz'); do \
			echo "==> $$pkg $$target"; \
			go test -run '^$$' -fuzz "^$$target\$$" -fuzztime $(FUZZTIME) $$pkg; \
		done; \
	done

# Build the provider
.PHONY: build
build:
	go build -o terraform-provider-fleetdm

# Install the provider locally for testing
.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/l-teles/fleetdm/0.1.0/$$(go env GOOS)_$$(go env GOARCH)
	mv terraform-provider-fleetdm ~/.terraform.d/plugins/registry.terraform.io/l-teles/fleetdm/0.1.0/$$(go env GOOS)_$$(go env GOARCH)/

# Generate documentation
.PHONY: docs
docs:
	go generate ./...

# Format code
.PHONY: fmt
fmt:
	gofmt -s -w .

# Lint code
.PHONY: lint
lint:
	golangci-lint run ./...

# Clean build artifacts
.PHONY: clean
clean:
	rm -f terraform-provider-fleetdm
	rm -rf ~/.terraform.d/plugins/registry.terraform.io/l-teles/fleetdm

# Verify go modules
.PHONY: verify
verify:
	go mod verify
	go mod tidy
