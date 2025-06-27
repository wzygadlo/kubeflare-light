# OpenCode.md - KubeFlare Light

## Build & Test Commands
- Build: `make build` or `go build -o kubeflare ./cmd/kubeflare`
- Test all: `go test ./...`
- Test single: `go test ./path/to/package -run TestName`
- Lint: `golangci-lint run`
- Format: `go fmt ./...`

## Code Style Guidelines
- **Imports**: Group standard library, third-party, and local imports
- **Formatting**: Follow Go standard formatting with `go fmt`
- **Types**: Use strong typing, prefer interfaces for flexibility
- **Naming**: Use camelCase for variables, PascalCase for exported functions/types
- **Error Handling**: Always check errors, use custom error types for domain-specific errors
- **Comments**: Document exported functions with godoc-style comments
- **Testing**: Write table-driven tests, use testify for assertions
- **Logging**: Use structured logging with levels (info, warn, error)

## Project Structure
- `/cmd`: Main applications
- `/pkg`: Library code
- `/internal`: Private application code
- `/api`: API definitions
- `/test`: Additional test files