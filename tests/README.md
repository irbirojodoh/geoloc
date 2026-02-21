# Geoloc Test Suite

This project follows idiomatic Go testing conventions by placing tests alongside the files they verify (i.e. inside `internal/auth`, `internal/data`, and `internal/handlers`). This allows test files to legally access unexported variables and package state tightly without exposing internal APIs to the public module space.

However, the tests are logically categorized into three distinct layers, mirrored logically as:

## 1. Unit Tests (`test-unit`)
Located in: `internal/auth/` and `internal/handlers/oauth_test.go`
- **Purpose**: Verify stateless pure-functions like JWT token generation, boundary math, and basic data mapping. Dependencies are mocked.

## 2. Integration Tests (`test-integration`)
Located in: `internal/data/`
- **Purpose**: Verify queries and database operations directly against a real Cassandra and Redis deployment.
- **Execution**: Automatically spins up required topology via `testcontainers-go` before running assertions.

## 3. End-to-End Tests (`test-e2e`)
Located in: `internal/handlers/e2e_test.go`
- **Purpose**: Validates complete HTTP endpoints via `httptest.NewRecorder()`. Simulates an end-user completing multiple chained lifecycle events (registering, getting tokens, making a post, commenting, searching).
- **Execution**: Uses `testcontainers-go` to stand up a real database to emulate realistic end-user flows seamlessly.

## Makefile Commands

*   `make test` - Run all test suites
*   `make test-unit` - Run only stateless unit tests
*   `make test-integration` - Run data accessibility tests
*   `make test-e2e` - Run rigorous server routing validations
