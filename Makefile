.PHONY: run test test-data docker-up

run:
	go run cmd/api/main.go

test:
	go test -v ./...

test-data:
	go test -v ./internal/data/...

docker-up:
	docker compose down -v
	docker compose up -d

check-cassandra-nodes:
	docker exec -it cassandra-1 nodetool status
