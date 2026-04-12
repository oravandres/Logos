.PHONY: verify test lint build install-hooks

verify: lint test build

lint:
	go vet ./...
	golangci-lint run ./...

test:
	go test ./... -count=1 -timeout 60s

build:
	go build -o logos ./cmd/logos

install-hooks:
	@echo "Installing pre-push hook..."
	@cp scripts/pre-push .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "Done. Pre-push hook will run lint + tests before every push."
