#!/usr/bin/env bash
# E2E test: publish a Swift package to OpenSPMRegistry (Maven-backed) and resolve it from a consumer.
# Prerequisites: Nexus running (make test-integration-up), Swift toolchain installed.
# Run from repository root.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

REGISTRY_URL="${E2E_REGISTRY_URL:-http://127.0.0.1:8082}"
E2E_USE_HTTPS=false
[[ "$REGISTRY_URL" == https://* ]] && E2E_USE_HTTPS=true
# HTTPS config uses admin/admin123 (passthrough to Nexus); HTTP has no auth
E2E_USER="${E2E_REGISTRY_USER:-admin}"
E2E_PASS="${E2E_REGISTRY_PASS:-admin123}"
SCOPE="example"
PACKAGE_NAME="SamplePackage"
VERSION="1.0.0"
[ "$E2E_USE_HTTPS" = true ] && CONFIG_E2E="config.e2e.https.yml" || CONFIG_E2E="config.e2e.yml"
SERVER_PID=""
SERVER_BINARY=""

cleanup() {
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		echo "Stopping OpenSPMRegistry (PID $SERVER_PID)..."
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	[ -n "$SERVER_BINARY" ] && [ -f "$SERVER_BINARY" ] && rm -f "$SERVER_BINARY"
}
trap cleanup EXIT

echo "=== E2E Swift Publish and Resolve ==="

# Clean state before test
echo "Cleaning Consumer directory..."
rm -f "$ROOT_DIR/testdata/e2e/Consumer/Package.resolved"
rm -rf "$ROOT_DIR/testdata/e2e/Consumer/.build"

echo "Cleaning example.SamplePackage from Nexus..."
"$SCRIPT_DIR/e2e-clean-nexus.sh" 2>/dev/null || true

if ! command -v swift >/dev/null 2>&1; then
	echo "Skipping E2E test: Swift toolchain not found. Install Swift to run this test."
	exit 0
fi

echo "Purging Swift PM cache and checksums..."
(cd "$ROOT_DIR/testdata/e2e/Consumer" && swift package purge-cache) 2>/dev/null || true
rm -rf "${HOME}/Library/Caches/org.swift.swiftpm" 2>/dev/null || true
rm -rf "${HOME}/Library/org.swift.swiftpm" 2>/dev/null || true
rm -rf "${HOME}/.cache/org.swift.swiftpm" 2>/dev/null || true

if ! curl -sf http://localhost:8081/service/rest/v1/status >/dev/null 2>&1; then
	echo "Nexus is not reachable at http://localhost:8081. Start it with: make test-integration-up"
	exit 1
fi

if [ ! -f "$CONFIG_E2E" ]; then
	echo "Missing $CONFIG_E2E"
	exit 1
fi

if [ "$E2E_USE_HTTPS" = true ]; then
	E2E_CERTS_DIR="$ROOT_DIR/testdata/e2e/certs"
	if [ ! -f "$E2E_CERTS_DIR/server.crt" ] || [ ! -f "$E2E_CERTS_DIR/server.key" ]; then
		echo "Generating E2E HTTPS certs..."
		"$SCRIPT_DIR/e2e-generate-certs.sh"
	fi
	echo "Adding E2E cert to keychain (for Swift PM to trust HTTPS)..."
	security add-trusted-cert -d -r trustRoot -p ssl "$E2E_CERTS_DIR/server.crt" 2>/dev/null || true
fi

echo "Building OpenSPMRegistry..."
go build -o openspmregistry.e2e main.go
SERVER_BINARY="$ROOT_DIR/openspmregistry.e2e"

echo "Starting OpenSPMRegistry with $CONFIG_E2E..."
"$SERVER_BINARY" -config "$CONFIG_E2E" -v &
SERVER_PID=$!

echo "Waiting for registry at $REGISTRY_URL..."
CURL_OPTS="-s --connect-timeout 2"
[ "$E2E_USE_HTTPS" = true ] && CURL_OPTS="$CURL_OPTS -k"
for i in $(seq 1 30); do
	if curl $CURL_OPTS "${REGISTRY_URL}/" >/dev/null 2>&1; then
		break
	fi
	if [ "$i" -eq 30 ]; then
		echo "Registry did not become ready in time."
		exit 1
	fi
	sleep 1
done
echo "Registry is ready."

if [ "$E2E_USE_HTTPS" = true ]; then
	echo "Logging in to registry (required for HTTPS + auth)..."
	swift package-registry login "$REGISTRY_URL" --username "$E2E_USER" --password "$E2E_PASS" --no-confirm || {
		echo "Tip: If login fails with keychain error, run interactively: swift package-registry login $REGISTRY_URL --username $E2E_USER --password $E2E_PASS"
		exit 1
	}
fi

PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
echo "Preparing sample package (Package.json, package-metadata.json, Package@swift-5.8.swift)..."
cd "$ROOT_DIR/testdata/e2e/example.SamplePackage"
swift package dump-package > Package.json 2>/dev/null || true

echo "Publishing $PACKAGE_ID $VERSION (with metadata and manifest variants)..."
PUBLISH_OPTS="--url $REGISTRY_URL"
[ "$E2E_USE_HTTPS" = false ] && PUBLISH_OPTS="$PUBLISH_OPTS --allow-insecure-http"
swift package-registry publish "$PACKAGE_ID" "$VERSION" $PUBLISH_OPTS

echo "Verifying package metadata..."
ACCEPT_JSON="Accept: application/vnd.swift.registry.v1+json"
VERIFY_AUTH=""
[ "$E2E_USE_HTTPS" = true ] && VERIFY_AUTH="-u ${E2E_USER}:${E2E_PASS}"
INFO_JSON=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}")
if ! echo "$INFO_JSON" | grep -q '"metadata"'; then
	echo "Package info response missing metadata."
	exit 1
fi
if ! echo "$INFO_JSON" | grep -q '"description"'; then
	echo "Package info metadata missing description."
	exit 1
fi

echo "Verifying alternative manifest (Package@swift-5.8)..."
ACCEPT_SWIFT="Accept: application/vnd.swift.registry.v1+swift"
MANIFEST_58=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_SWIFT" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}/Package.swift?swift-version=5.8")
if ! echo "$MANIFEST_58" | grep -q "swift-tools-version:5.8"; then
	echo "Package@swift-5.8 manifest not found or wrong version."
	exit 1
fi

echo "Configuring consumer to use registry and resolving..."
cd "$ROOT_DIR/testdata/e2e/Consumer"
if [ "$E2E_USE_HTTPS" = true ]; then
	swift package-registry set "$REGISTRY_URL"
else
	swift package-registry set "$REGISTRY_URL" --allow-insecure-http
fi
swift package resolve

if [ ! -f Package.resolved ]; then
	echo "Package.resolved was not created; resolve may have failed."
	exit 1
fi
if ! grep -q "example.SamplePackage" Package.resolved; then
	echo "Package.resolved does not contain example.SamplePackage."
	exit 1
fi

echo "E2E Swift publish and resolve: OK"
