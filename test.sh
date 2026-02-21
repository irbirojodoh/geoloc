#!/bin/bash

# Ensure the script stops on the first error
set -e

# Default to running all tests if no argument is provided
TEST_TYPE=${1:-all}

echo "=========================================="
echo "🧪 Running Geoloc Test Suite: $TEST_TYPE"
echo "=========================================="

case "$TEST_TYPE" in
    unit)
        echo "--> Running Unit Tests..."
        make test-unit
        ;;
    integration)
        echo "--> Running Integration Tests (requires Docker for testcontainers)..."
        make test-integration
        ;;
    e2e)
        echo "--> Running End-to-End Tests (requires Docker for testcontainers)..."
        make test-e2e
        ;;
    all)
        echo "--> Running ALL Tests (Unit, Integration, E2E)..."
        make test
        ;;
    *)
        echo "❌ Invalid argument: $TEST_TYPE"
        echo "Usage: ./test.sh [unit|integration|e2e|all]"
        exit 1
        ;;
esac

echo "✅ All requested $TEST_TYPE tests passed successfully!"
