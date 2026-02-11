# Integration Tests

This project includes integration tests that use a real Maven repository server (Nexus OSS) to test Maven repository functionality.

## Prerequisites

- Docker and Docker Compose installed
- Go 1.23+

## Quick Start

### Run Integration Tests

```bash
make test-integration
```

This will:
1. Start Nexus OSS in a Docker container (embedded DB, no separate database)
2. Wait for Nexus to be ready (2–3 minutes on first start)
3. Run the bootstrap script: create Maven repo `private` via Script API, set admin password to `admin123`
4. Run integration tests
5. Stop and clean up the container

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
INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8081/repository MAVEN_REPO_NAME=private MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD=admin123 go test -tags=integration -v ./repo/maven/... -run TestIntegration
```

## Architecture

### Docker Compose

The `docker-compose.test.yml` file defines:
- **Nexus OSS**: Maven repository server (embedded database, no separate DB container)
  - Port: 8081
  - Data persisted in local directory `./nexus-data/` for easy debugging
  - Image: `sonatype/nexus3:3.68.1` (security-hardened; scripting enabled via `INSTALL4J_ADD_VM_PARAMS` for bootstrap)
  - Container name: `nexus-test`

### Bootstrap Script

After Nexus is ready, `scripts/nexus-bootstrap.sh`:
- Reads the initial admin password from the container (`/nexus-data/admin.password`) if present
- Creates a Maven hosted repository named `private` via the Nexus Script API
- Sets the admin password to `admin123` so tests can use fixed credentials

The script is idempotent (safe to run multiple times).

### Scope and package listing

`ListScopes` and `ListInScope` use the SPM registry index only (path `com/spm/registry/index/1/index-1.json`, Maven 2 layout); there are no directory/HTML fallbacks. The index contains `scopes` (array) and `packages` (map from scope to package names). If the file is missing or invalid, or a scope has no packages in the index, the respective call returns an empty list. Publishing from this codebase updates both `scopes` and `packages` in the index so it stays in sync.

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
        baseURL = "http://localhost:8081/repository"
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
- `MAVEN_REPO_URL`: Base URL of the Maven repository (default: `http://localhost:8081/repository`)
- `MAVEN_REPO_NAME`: Repository name (default: `private`, created by bootstrap script)
- `MAVEN_REPO_USERNAME`: Username for Maven repository authentication (default: `admin`)
- `MAVEN_REPO_PASSWORD`: Password for Maven repository authentication (default: `admin123`)

### Nexus OSS Defaults

- URL: http://localhost:8081/repository/private (base + repo name)
- Default repository: `private` (created by `scripts/nexus-bootstrap.sh`)
- Default credentials: `admin` / `admin123` (password set by bootstrap)
- Data directory: `./nexus-data/` (host); `/nexus-data` in container
- Image: `sonatype/nexus3:3.68.1` (scripting enabled via env for repo creation)

The integration tests append the repository name to the base URL if not already present. Customize via `MAVEN_REPO_NAME`.

### Bootstrap Script Environment

The script `scripts/nexus-bootstrap.sh` accepts:
- `NEXUS_URL`: Base URL (default: `http://localhost:8081`)
- `NEXUS_CONTAINER`: Container name (default: `nexus-test`)
- `NEXUS_REPO_KEY`: Repository key to create (default: `private`)
- `NEXUS_TARGET_PASSWORD`: Admin password to set (default: `admin123`)

## Troubleshooting

### Server Not Starting

Check if the port is already in use:
```bash
lsof -i :8081
```

### Tests Timing Out

Increase the timeout in your test:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
```

### Clean Up

To completely remove test data and containers:
```bash
docker-compose -f docker-compose.test.yml down
rm -rf nexus-data/
```

## CI/CD Integration

Integration tests are designed to be run in CI/CD pipelines. The Docker Compose setup ensures:
- Isolated test environment
- Bootstrap creates repo and sets password before tests
- Health checks before running tests

Example GitHub Actions step:
```yaml
- name: Run integration tests
  run: make test-integration
```
