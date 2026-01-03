.PHONY: run test test-data test-auth docker-up

run:
	go run cmd/api/main.go

test:
	go test -v -count=1 ./...

test-auth:
	go test -v -count=1 ./internal/auth/...

test-data:
	go test -v -count=1 ./internal/data/...

docker-up:
	docker compose down -v
	docker compose up -d

check-cassandra-nodes:
	docker exec -it cassandra-1 nodetool status
