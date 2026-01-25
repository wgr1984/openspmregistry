# Integration Tests

This project includes integration tests that use a real Maven repository server (Reposilite) to test Maven repository functionality.

## Prerequisites

- Docker and Docker Compose installed
- Go 1.23+

## Quick Start

### Run Integration Tests

```bash
make test-integration
```

This will:
1. Start Reposilite in a Docker container
2. Wait for it to be ready
3. Run integration tests
4. Stop and clean up the container

### Manual Control

Start the test server:
```bash
make test-integration-up
```

Stop the test server:
```bash
make test-integration-down
```

Run integration tests manually:
```bash
INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8080 go test -tags=integration -v ./repo/maven/... -run TestIntegration
```

## Architecture

### Docker Compose

The `docker-compose.test.yml` file defines:
- **Reposilite**: Lightweight Maven repository server
  - Port: 8080
  - Data persisted in local directory `./maven-files/` for easy debugging
  - Admin token configured via `ADMIN_TOKEN` environment variable (default: `admin:admin123`)

### Integration Test Helper

The `integration_test.go` file provides:
- `IntegrationTestHelper`: Utilities for integration tests
- `WaitForServer()`: Waits for the Maven server to be ready
- `GetMavenConfig()`: Returns a configured MavenConfig for tests
- `SkipIfNotIntegration()`: Skips tests if integration mode is not enabled

## Writing Integration Tests

Integration tests should:
1. Use the `integration` build tag
2. Call `SkipIfNotIntegration(t)` at the start
3. Use the `IntegrationTestHelper` to set up the test environment

Example:

```go
//go:build integration
// +build integration

func TestMyIntegrationTest(t *testing.T) {
    SkipIfNotIntegration(t)
    
    baseURL := os.Getenv("MAVEN_REPO_URL")
    if baseURL == "" {
        baseURL = "http://localhost:8080"
    }
    
    helper := NewIntegrationTestHelper(baseURL)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    
    if err := helper.WaitForServer(ctx, 2*time.Minute); err != nil {
        t.Fatalf("Server not ready: %v", err)
    }
    
    cfg := helper.GetMavenConfig()
    repo, err := NewMavenRepo(cfg)
    // ... your test code
}
```

## Configuration

### Environment Variables

- `INTEGRATION_TESTS`: Set to `1` to enable integration tests
- `MAVEN_REPO_URL`: Base URL of the Maven repository (default: `http://localhost:8080`)
- `MAVEN_REPO_NAME`: Repository name in Reposilite (default: `private`)
- `MAVEN_REPO_USERNAME`: Username for Maven repository authentication (default: `admin`)
- `MAVEN_REPO_PASSWORD`: Password for Maven repository authentication (default: `admin123`)

### Reposilite Defaults

- URL: http://localhost:8080
- Default repository: `private` (created automatically on first upload)
- Default admin token: `admin:admin123` (configured via `ADMIN_TOKEN` environment variable)
- Data directory: `./maven-files/` (local directory for easy debugging)
- Image: `dzikoysk/reposilite:3.5.0`

**Note:** Reposilite requires a repository name prefix in the URL. The integration tests automatically append `/private` to the BaseURL if not already present. You can customize this via the `MAVEN_REPO_NAME` environment variable.

### Customizing Admin Token

You can customize the admin token by setting the `ADMIN_TOKEN` environment variable:
```bash
ADMIN_TOKEN=myuser:mypassword make test-integration-up
```

## Troubleshooting

### Server Not Starting

Check if the port is already in use:
```bash
lsof -i :8080
```

### Tests Timing Out

Increase the timeout in your test:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
```

### Clean Up

To completely remove test data:
```bash
docker-compose -f docker-compose.test.yml down
rm -rf maven-files/
```

## CI/CD Integration

Integration tests are designed to be run in CI/CD pipelines. The Docker Compose setup ensures:
- Isolated test environment
- Automatic cleanup
- Health checks before running tests

Example GitHub Actions step:
```yaml
- name: Run integration tests
  run: make test-integration
```
