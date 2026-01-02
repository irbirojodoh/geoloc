.PHONY: run test test-data

run:
	go run cmd/api/main.go

test:
	go test -v ./...

test-data:
	go test -v ./internal/data/...
