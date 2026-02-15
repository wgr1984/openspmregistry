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
# Packages: name -> directory
# SamplePackage: 1.0.0, 1.1.0
# UtilsPackage: 1.0.0, 1.1.0
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

echo "Cleaning E2E packages (example.SamplePackage, example.UtilsPackage) from Nexus..."
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

# Free port in case a previous run's server didn't exit cleanly
if command -v lsof >/dev/null 2>&1; then
	lsof -ti :8082 | xargs kill -9 2>/dev/null || true
	sleep 1
fi

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

ACCEPT_JSON="Accept: application/vnd.swift.registry.v1+json"
VERIFY_AUTH=""
[ "$E2E_USE_HTTPS" = true ] && VERIFY_AUTH="-u ${E2E_USER}:${E2E_PASS}"
PUBLISH_OPTS="--url $REGISTRY_URL"
[ "$E2E_USE_HTTPS" = false ] && PUBLISH_OPTS="$PUBLISH_OPTS --allow-insecure-http"

# Publish SamplePackage 1.0.0 and 1.1.0
for VERSION in 1.0.0 1.1.0; do
	PACKAGE_NAME="SamplePackage"
	PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
	echo "Preparing $PACKAGE_ID (Package.json, package-metadata.json, Package@swift-5.8.swift)..."
	cd "$ROOT_DIR/testdata/e2e/example.SamplePackage"
	swift package dump-package > Package.json 2>/dev/null || true
	echo "Publishing $PACKAGE_ID $VERSION..."
	swift package-registry publish "$PACKAGE_ID" "$VERSION" $PUBLISH_OPTS
done

# Publish UtilsPackage 1.0.0 and 1.1.0
for VERSION in 1.0.0 1.1.0; do
	PACKAGE_NAME="UtilsPackage"
	PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
	echo "Preparing $PACKAGE_ID..."
	cd "$ROOT_DIR/testdata/e2e/example.UtilsPackage"
	swift package dump-package > Package.json 2>/dev/null || true
	echo "Publishing $PACKAGE_ID $VERSION..."
	swift package-registry publish "$PACKAGE_ID" "$VERSION" $PUBLISH_OPTS
done

# Verify package metadata and manifest for SamplePackage 1.0.0
PACKAGE_NAME="SamplePackage"
VERSION="1.0.0"
PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
echo "Verifying $PACKAGE_ID $VERSION metadata..."
INFO_JSON=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}")
if ! echo "$INFO_JSON" | grep -q '"metadata"'; then
	echo "Package info response missing metadata."
	exit 1
fi
if ! echo "$INFO_JSON" | grep -q '"description"'; then
	echo "Package info metadata missing description."
	exit 1
fi
echo "  OK: package metadata"

echo "Verifying alternative manifest (Package@swift-5.8) for $PACKAGE_ID..."
ACCEPT_SWIFT="Accept: application/vnd.swift.registry.v1+swift"
MANIFEST_58=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_SWIFT" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}/Package.swift?swift-version=5.8")
if ! echo "$MANIFEST_58" | grep -q "swift-tools-version:5.8"; then
	echo "Package@swift-5.8 manifest not found or wrong version."
	exit 1
fi
echo "  OK: alternative manifest"

echo "Verifying package collection (global)..."
COLLECTION_GLOBAL=$(curl $CURL_OPTS $VERIFY_AUTH -H "Accept: application/json" "${REGISTRY_URL}/collection")
if ! echo "$COLLECTION_GLOBAL" | grep -q '"formatVersion"'; then
	echo "Global collection response missing formatVersion."
	exit 1
fi
if ! echo "$COLLECTION_GLOBAL" | grep -q '"packages"'; then
	echo "Global collection response missing packages array."
	exit 1
fi
for PKG in example.SamplePackage example.UtilsPackage; do
	if ! echo "$COLLECTION_GLOBAL" | grep -q "\"${PKG}\""; then
		echo "Global collection does not contain ${PKG}."
		exit 1
	fi
done
if ! echo "$COLLECTION_GLOBAL" | grep -q '"generatedBy"'; then
	echo "Global collection response missing generatedBy."
	exit 1
fi
echo "  OK: global collection (contains both packages)"

echo "Verifying package collection (scope ${SCOPE})..."
COLLECTION_SCOPE=$(curl $CURL_OPTS $VERIFY_AUTH -H "Accept: application/json" "${REGISTRY_URL}/collection/${SCOPE}")
for PKG_ID in example.SamplePackage example.UtilsPackage; do
	if ! echo "$COLLECTION_SCOPE" | grep -q "\"${PKG_ID}\""; then
		echo "Scope collection /collection/${SCOPE} does not contain ${PKG_ID}."
		exit 1
	fi
done
for VER in 1.0.0 1.1.0; do
	if ! echo "$COLLECTION_SCOPE" | grep -q "\"${VER}\""; then
		echo "Scope collection does not contain version ${VER}."
		exit 1
	fi
done
echo "  OK: scope collection (both packages, multiple versions)"

echo "Verifying package collection (non-existent scope returns 404)..."
COLLECTION_404=$(curl $CURL_OPTS $VERIFY_AUTH -w "%{http_code}" -o /dev/null -H "Accept: application/json" "${REGISTRY_URL}/collection/nonexistentscope123")
if [ "$COLLECTION_404" != "404" ]; then
	echo "Expected 404 for non-existent scope, got ${COLLECTION_404}."
	exit 1
fi
echo "  OK: 404 for non-existent scope"

echo "Verifying Swift package-collection CLI (add, list, describe)..."
COLLECTION_FILE=$(mktemp)
curl $CURL_OPTS $VERIFY_AUTH -H "Accept: application/json" "${REGISTRY_URL}/collection" -o "$COLLECTION_FILE"
# Swift CLI only accepts https or file URLs; use file:// for HTTP, direct URL for HTTPS
# For HTTPS with auth: use ?auth=<base64(full Authorization header)> since Swift cannot send headers
if [ "$E2E_USE_HTTPS" = true ]; then
	BASIC_HEADER="Basic $(echo -n "${E2E_USER}:${E2E_PASS}" | base64)"
	AUTH_B64=$(echo -n "$BASIC_HEADER" | base64)
	COLLECTION_ADD_URL="${REGISTRY_URL}/collection?auth=${AUTH_B64}"
else
	COLLECTION_ADD_URL="file://${COLLECTION_FILE}"
fi
# Remove if already added (from previous run)
swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
swift package-collection add "$COLLECTION_ADD_URL" --trust-unsigned || {
	rm -f "$COLLECTION_FILE"
	echo "swift package-collection add failed."
	exit 1
}
echo "  OK: collection add"
if ! swift package-collection list 2>/dev/null | grep -q "All Packages"; then
	rm -f "$COLLECTION_FILE"
	swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
	echo "swift package-collection list: collection not found."
	exit 1
fi
echo "  OK: collection list"
if ! swift package-collection describe "$COLLECTION_ADD_URL" 2>/dev/null | grep -qi "example"; then
	rm -f "$COLLECTION_FILE"
	swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
	echo "swift package-collection describe: package not found in collection."
	exit 1
fi
echo "  OK: collection describe"
swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
rm -f "$COLLECTION_FILE"

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
for PKG in example.SamplePackage example.UtilsPackage; do
	if ! grep -q "$PKG" Package.resolved; then
		echo "Package.resolved does not contain $PKG."
		exit 1
	fi
done
echo "  OK: consumer resolve (both packages)"

echo "Building and running Consumer..."
swift build
OUTPUT=$(swift run Consumer 2>&1)
if ! echo "$OUTPUT" | grep -q "Resolved SamplePackage"; then
	echo "Consumer output missing SamplePackage: $OUTPUT"
	exit 1
fi
if ! echo "$OUTPUT" | grep -q "Resolved UtilsPackage"; then
	echo "Consumer output missing UtilsPackage: $OUTPUT"
	exit 1
fi
echo "  OK: consumer build and run"

echo ""
echo "E2E Swift publish and resolve: OK"
