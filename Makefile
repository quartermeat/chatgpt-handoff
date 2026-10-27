.PHONY: ci install
ci:
	@test -z "$$(gofmt -l *.go)" || (gofmt -l *.go; exit 1)
	go vet ./...
	go test ./...
install:
	@mkdir -p "$(HOME)/.local/bin"
	go build -o "$(HOME)/.local/bin/chatgpt-handoff" .
