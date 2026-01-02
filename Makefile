.PHONY: run test

run:
	go run cmd/api/main.go

test:
	go test -v ./...
