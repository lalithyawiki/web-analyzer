.PHONY: test
test:
	@echo "Running all tests..."
	@go test -v ./...

.PHONY: coverage
coverage:
	@echo "Generating test coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out