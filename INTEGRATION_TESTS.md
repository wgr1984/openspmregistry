# Integration Tests

This project includes integration tests that use a real Maven repository server (Nexus OSS or Reposilite) to test Maven repository functionality.

## Prerequisites

- Docker and Docker Compose installed
- Go 1.23+

## Maven provider switch

Set **MAVEN_PROVIDER** to choose the backend:

- **nexus** (default): Nexus OSS on port 8081, repo `private`, bootstrap creates repo and sets admin password.
- **reposilite**: Reposilite on port 8080, repo `private`, bootstrap writes token secret to `.reposilite-test-token`.

Examples:

```bash
make test-integration                    # Nexus (default)
make test-integration MAVEN_PROVIDER=reposilite
make test-e2e-full MAVEN_PROVIDER=reposilite
```

## Quick Start

### Run Integration Tests

```bash
make test-integration
```

This will (with default **MAVEN_PROVIDER=nexus**):
1. Start Nexus OSS in a Docker container (embedded DB, no separate database)
2. Wait for Nexus to be ready (2–3 minutes on first start)
3. Run the bootstrap script: create Maven repo `private` via Script API, set admin password to `admin123`
4. Run integration tests
5. Stop and clean up the container

With **MAVEN_PROVIDER=reposilite** the Makefile starts Reposilite, waits for it, runs reposilite-bootstrap (writes token to `.reposilite-test-token`), then runs the same integration tests against `http://localhost:8080/private`.

### Manual Control

Start the test server:
```bash
make test-integration-up
# or
MAVEN_PROVIDER=reposilite make test-integration-up
```

Stop the test server:
```bash
make test-integration-down
```

Run integration tests manually (Nexus):
```bash
INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8081/repository MAVEN_REPO_NAME=private MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD=admin123 go test -tags=integration -v ./repo/maven/... -run TestIntegration
```

Run integration tests manually (Reposilite):
```bash
INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8080 MAVEN_REPO_NAME=private MAVEN_PROVIDER=reposilite MAVEN_REPO_USERNAME=e2e MAVEN_REPO_PASSWORD=test-secret go test -tags=integration -v ./repo/maven/... -run TestIntegration
```

## E2E Swift Publish and Resolve

A real end-to-end test uses the Swift CLI to publish a package to OpenSPMRegistry (backed by Maven: Nexus or Reposilite) and resolve it from a consumer project.

### Prerequisites

- Maven server running (same as integration tests; start with `make test-integration-up` or `MAVEN_PROVIDER=reposilite make test-integration-up`)
- Swift toolchain installed (Swift 5.9+ recommended)
- Run from repository root

### How to Run

**Option 1 — Maven server already up** (e.g. after `make test-integration-up`):

```bash
make test-e2e-swift
# or with Reposilite:
MAVEN_PROVIDER=reposilite make test-e2e-swift
```

**Option 2 — Start Maven server, run E2E, then tear down:**

```bash
make test-e2e-full
MAVEN_PROVIDER=reposilite make test-e2e-full
```

If Swift is not installed, `test-e2e-swift` exits successfully without failing (skip).

### What It Does

1. Cleans state: removes Consumer `Package.resolved` and `.build`, purges Swift PM cache, deletes E2E packages from the Maven repo (in-test cleanup; or `go run ./cmd/e2e-clean-nexus` with MAVEN_PROVIDER set).
2. Builds OpenSPMRegistry and starts it with `config.e2e.yml` (Nexus) or `config.e2e.reposilite.yml` (Reposilite); HTTP on port 8082.
3. Prepares the sample package: generates `Package.json` (via `swift package dump-package`), uses `package-metadata.json` (description, author, license), and includes `Package@swift-5.8.swift` for manifest variants.
4. Publishes `example.SamplePackage` version `1.0.0` via `swift package-registry publish`.
5. Verifies package metadata (GET package info, checks `metadata.description`).
6. Verifies alternative manifest (GET `Package.swift?swift-version=5.8`, checks `swift-tools-version:5.8`).
7. Verifies package collections (curl + Swift CLI):
   - GET `/collection`: checks formatVersion, packages array, generatedBy, and that `example.SamplePackage` is included.
   - GET `/collection/example`: checks scope-specific collection contains the package and version.
   - GET `/collection/nonexistentscope123`: expects 404 for non-existent scope.
   - Swift CLI: `swift package-collection add` (uses file:// for HTTP, direct URL for HTTPS), `list`, `describe`, then `remove`.
8. In `testdata/e2e/Consumer/`, configures the registry and runs `swift package resolve`.
9. Verifies that `Package.resolved` contains `example.SamplePackage`.

### Sample Package Structure

The E2E sample package (`testdata/e2e/example.SamplePackage/`) includes:

- **Package.swift** (swift-tools-version:5.9): Base manifest
- **Package@swift-5.8.swift**: Alternative manifest for Swift 5.8
- **package-metadata.json**: Release metadata (description, author, licenseURL, repositoryURLs) per SPM Registry spec
- **Package.json**: Generated at runtime via `swift package dump-package` (used for package collections)

### Configuration

- **config.e2e.yml**: E2E server config for Nexus (port 8082, HTTP, Maven repo `http://localhost:8081/repository/private`, admin/admin123).
- **config.e2e.reposilite.yml**: E2E server config for Reposilite (Maven repo `http://localhost:8080/private`, e2e/token from `.reposilite-test-token`).
- **E2E_REGISTRY_URL**: Override registry URL (default: `http://127.0.0.1:8082`).
- **MAVEN_PROVIDER**: `nexus` (default) or `reposilite`; selects config and cleanup/health logic.

### HTTPS E2E

For HTTPS + auth testing, the script handles cert generation, keychain trust, and login automatically:

```bash
E2E_REGISTRY_URL=https://127.0.0.1:8082 make test-e2e-swift
```

This uses `config.e2e.https.yml`, generates certs if needed, adds them to the keychain, starts the registry, runs `swift package-registry login` (while server is running), then publishes and resolves.

## Architecture

### Docker Compose

The `docker-compose.test.yml` file defines two services; the Makefile starts one based on **MAVEN_PROVIDER**:

- **Nexus OSS**: Maven repository server (embedded database, no separate DB container)
  - Port: 8081
  - Data persisted in local directory `./nexus-data/`
  - Image: `sonatype/nexus3:3.68.1` (scripting enabled for bootstrap)
  - Container name: `nexus-test`

- **Reposilite**: Lightweight Maven repository manager
  - Port: 8080
  - Data persisted in `./reposilite-data/`
  - Image: `dzikoysk/reposilite:3.5.28`, started with `--token e2e:test-secret` for E2E auth
  - Container name: `reposilite-test`
  - Default repo `private` is used for tests

### Bootstrap

- **Nexus** (when MAVEN_PROVIDER=nexus): `go run ./cmd/nexus-bootstrap` reads the initial admin password from the container, creates a Maven hosted repository `private` via the Nexus Script API, sets admin password to `admin123`, and optionally writes it to `.nexus-test-password`.
- **Reposilite** (when MAVEN_PROVIDER=reposilite): `go run ./cmd/reposilite-bootstrap` verifies the server is up and writes the test token secret to `.reposilite-test-token` (token name `e2e`, secret from env or default `test-secret`; must match the `--token` used in Docker).

Both bootstrap scripts are idempotent.

### Scope and package listing

`ListScopes` and `ListInScope` use the SPM registry index only (path `com/spm/registry/index/1/index-1.json`, Maven 2 layout); there are no directory/HTML fallbacks. The index contains `packages` (map from scope to package names). Scopes are derived from `packages` keys when reading. If the file is missing or invalid, or a scope has no packages in the index, the respective call returns an empty list. Publishing from this codebase updates `packages` in the index so it stays in sync.

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
- `MAVEN_PROVIDER`: `nexus` (default) or `reposilite`
- `MAVEN_REPO_URL`: Base URL of the Maven repository (Nexus: `http://localhost:8081/repository`; Reposilite: `http://localhost:8080`)
- `MAVEN_REPO_NAME`: Repository name (Nexus and Reposilite: `private`)
- `MAVEN_REPO_USERNAME`: Username (Nexus: `admin`; Reposilite: `e2e` token name)
- `MAVEN_REPO_PASSWORD`: Password or token secret (Nexus: from bootstrap or `admin123`; Reposilite: from `.reposilite-test-token` or `test-secret`)
- `NEXUS_URL`: Nexus base URL for API/health (default: `http://localhost:8081`); used when MAVEN_PROVIDER=nexus

### Nexus OSS Defaults

- URL: http://localhost:8081/repository/private (base + repo name)
- Default repository: `private` (created by `cmd/nexus-bootstrap`)
- Default credentials: `admin` / `admin123` (password set by bootstrap)
- Data directory: `./nexus-data/` (host); `/nexus-data` in container
- Image: `sonatype/nexus3:3.68.1` (scripting enabled via env for repo creation)

### Reposilite Defaults

- URL: http://localhost:8080/private
- Default repository: `private` (built-in)
- Test token: name `e2e`, secret in `.reposilite-test-token` (written by `cmd/reposilite-bootstrap`)
- Data directory: `./reposilite-data/`
- Image: `dzikoysk/reposilite:3.5.28` with `--token e2e:test-secret`

### Bootstrap Environment

`cmd/nexus-bootstrap` accepts:
- `NEXUS_URL`: Base URL (default: `http://localhost:8081`)
- `NEXUS_CONTAINER`: Container name (default: `nexus-test`)
- `NEXUS_REPO_KEY`: Repository key to create (default: `private`)
- `NEXUS_TARGET_PASSWORD`: Admin password to set (default: `admin123`)

`cmd/reposilite-bootstrap` accepts:
- `REPOSILITE_URL`: Base URL (default: `http://localhost:8080`)
- `REPOSILITE_TEST_TOKEN_FILE`: Where to write the token secret (default: `.reposilite-test-token`)
- `REPOSILITE_TEST_TOKEN_SECRET`: Token secret to write (default: `test-secret`)

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

On GitHub Actions, the Makefile uses a longer Nexus wait (6 minutes) because runners can be slow; locally 3 minutes is usually enough.

**Alternatives to running Nexus in CI on every push:**
- Run integration tests **locally**: `make test-integration` (recommended before merging Maven/nexus changes).
- Run in CI only on **schedule** or **workflow_dispatch**: change the workflow `on:` to `schedule: - cron: '0 2 * * *'` and/or `workflow_dispatch:` so they don't block every PR.
- Keep the current setup with the increased timeout; re-run the job if it occasionally times out.

Example GitHub Actions step:
```yaml
- name: Run integration tests
  run: make test-integration
  timeout-minutes: 15
```
