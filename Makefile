.PHONY: run build test test-auth test-data test-handlers lint docker-up docker-down docker-build logs check-cassandra-nodes

# ============== DEVELOPMENT ==============

run:
	go run cmd/api/main.go

build:
	docker build -t geoloc-api:latest .

# ============== TESTING ==============

test:
	JWT_SECRET=test-secret SESSION_SECRET=test-session go test -v -count=1 ./...

test-auth:
	JWT_SECRET=test-secret SESSION_SECRET=test-session go test -v -count=1 ./internal/auth/...

test-data:
	JWT_SECRET=test-secret SESSION_SECRET=test-session go test -v -count=1 ./internal/data/...

test-handlers:
	JWT_SECRET=test-secret SESSION_SECRET=test-session go test -v -count=1 ./internal/handlers/...

test-unit: test-auth
test-integration: test-data
test-e2e: test-handlers

# ============== LINTING ==============

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...

# ============== DOCKER ==============

docker-up:
	docker compose down -v
	docker compose up -d

docker-down:
	docker compose down

docker-build:
	docker compose build --no-cache api

# ============== MONITORING ==============

logs:
	docker compose logs -f api

logs-all:
	docker compose logs -f

check-cassandra-nodes:
	docker exec -it cassandra-1 nodetool status

health:
	curl -s http://localhost:8080/health | python3 -m json.tool
