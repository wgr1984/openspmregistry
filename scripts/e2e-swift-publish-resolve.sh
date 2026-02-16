#!/usr/bin/env bash
# E2E test: publish a Swift package to OpenSPMRegistry (Maven-backed) and resolve it from a consumer.
# Prerequisites: Nexus running (make test-integration-up), Swift toolchain installed.
# Run from repository root.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# Colors (disable if NO_COLOR set or not a TTY)
if [ -n "$NO_COLOR" ] || [ ! -t 1 ]; then
	C_RESET=""
	C_BOLD=""
	C_RED=""
	C_GREEN=""
	C_YELLOW=""
	C_CYAN=""
	C_BLUE=""
	C_DIM=""
else
	C_RESET="\033[0m"
	C_BOLD="\033[1m"
	C_RED="\033[31m"
	C_GREEN="\033[32m"
	C_YELLOW="\033[33m"
	C_CYAN="\033[36m"
	C_BLUE="\033[34m"
	C_DIM="\033[2m"
fi

section() {
	echo ""
	printf "${C_BOLD}${C_CYAN}╭─────────────────────────────────────────────────────────────────╮${C_RESET}\n"
	printf "${C_BOLD}${C_CYAN}│ %-63s │${C_RESET}\n" "$1"
	printf "${C_BOLD}${C_CYAN}╰─────────────────────────────────────────────────────────────────╯${C_RESET}\n"
}

ok() {
	printf "  ${C_GREEN}✓${C_RESET} %s\n" "$1"
}

info() {
	printf "  ${C_BLUE}▸${C_RESET} %s\n" "$1"
}

step() {
	printf "  ${C_DIM}%s${C_RESET}\n" "$1"
}

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
SERVER_LOG=""
E2E_HAD_WARNING=0

cleanup() {
	local exit_code="${1:-0}"
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		echo "Stopping OpenSPMRegistry (PID $SERVER_PID)..."
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	if [ "$exit_code" != 0 ] || [ "$E2E_HAD_WARNING" = 1 ] || [ -n "${E2E_VERBOSE:-}" ]; then
		[ -n "$SERVER_LOG" ] && [ -f "$SERVER_LOG" ] && {
			echo ""
			printf '%b\n' "${C_BOLD}${C_YELLOW}--- Server log ---${C_RESET}"
			cat "$SERVER_LOG"
		}
	fi
	[ -n "$SERVER_BINARY" ] && [ -f "$SERVER_BINARY" ] && rm -f "$SERVER_BINARY"
	[ -n "$SERVER_LOG" ] && [ -f "$SERVER_LOG" ] && rm -f "$SERVER_LOG"
}
trap 'cleanup $?' EXIT

section "E2E Swift Publish and Resolve"

# Clean state before test
section "Setup"
info "Cleaning Consumer directory..."
rm -f "$ROOT_DIR/testdata/e2e/Consumer/Package.resolved"
rm -rf "$ROOT_DIR/testdata/e2e/Consumer/.build"

info "Cleaning E2E packages (example.SamplePackage, example.UtilsPackage) from Nexus..."
"$SCRIPT_DIR/e2e-clean-nexus.sh" 2>/dev/null || true

if ! command -v swift >/dev/null 2>&1; then
	printf "${C_YELLOW}Skipping E2E test: Swift toolchain not found. Install Swift to run this test.${C_RESET}\n"
	exit 0
fi

info "Purging Swift PM cache and checksums..."
(cd "$ROOT_DIR/testdata/e2e/Consumer" && swift package purge-cache) 2>/dev/null || true
rm -rf "${HOME}/Library/Caches/org.swift.swiftpm" 2>/dev/null || true
rm -rf "${HOME}/Library/org.swift.swiftpm" 2>/dev/null || true
rm -rf "${HOME}/.cache/org.swift.swiftpm" 2>/dev/null || true

if ! curl -sf http://localhost:8081/service/rest/v1/status >/dev/null 2>&1; then
	printf "${C_RED}Nexus is not reachable at http://localhost:8081. Start it with: make test-integration-up${C_RESET}\n"
	exit 1
fi

if [ ! -f "$CONFIG_E2E" ]; then
	printf "${C_RED}Missing $CONFIG_E2E${C_RESET}\n"
	exit 1
fi

ok "Nexus reachable, config OK"

if [ "$E2E_USE_HTTPS" = true ]; then
	E2E_CERTS_DIR="$ROOT_DIR/testdata/e2e/certs"
	if [ ! -f "$E2E_CERTS_DIR/server.crt" ] || [ ! -f "$E2E_CERTS_DIR/server.key" ]; then
		info "Generating E2E HTTPS certs..."
		"$SCRIPT_DIR/e2e-generate-certs.sh"
	fi
	info "Adding E2E cert to keychain (for Swift PM to trust HTTPS)..."
	security add-trusted-cert -d -r trustRoot -p ssl "$E2E_CERTS_DIR/server.crt" 2>/dev/null || true
fi

info "Building OpenSPMRegistry..."
go build -o openspmregistry.e2e main.go && ok "Build complete"

SERVER_BINARY="$ROOT_DIR/openspmregistry.e2e"

# Free port in case a previous run's server didn't exit cleanly
if command -v lsof >/dev/null 2>&1; then
	lsof -ti :8082 | xargs kill -9 2>/dev/null || true
	sleep 1
fi

info "Starting OpenSPMRegistry with $CONFIG_E2E..."
SERVER_LOG=$(mktemp)
"$SERVER_BINARY" -config "$CONFIG_E2E" -v >> "$SERVER_LOG" 2>&1 &
SERVER_PID=$!

info "Waiting for registry at $REGISTRY_URL..."
CURL_OPTS="-s --connect-timeout 2"
[ "$E2E_USE_HTTPS" = true ] && CURL_OPTS="$CURL_OPTS -k"
for i in $(seq 1 30); do
	if curl $CURL_OPTS "${REGISTRY_URL}/" >/dev/null 2>&1; then
		break
	fi
	if [ "$i" -eq 30 ]; then
		printf "${C_RED}Registry did not become ready in time.${C_RESET}\n"
		exit 1
	fi
	sleep 1
done
ok "Registry is ready at $REGISTRY_URL"

section "Publish"
if [ "$E2E_USE_HTTPS" = true ]; then
	info "Logging in to registry (required for HTTPS + auth)..."
	swift package-registry login "$REGISTRY_URL" --username "$E2E_USER" --password "$E2E_PASS" --no-confirm || {
		printf "${C_YELLOW}Tip: If login fails with keychain error, run interactively: swift package-registry login $REGISTRY_URL --username $E2E_USER --password $E2E_PASS${C_RESET}\n"
		exit 1
	}
fi

ACCEPT_JSON="Accept: application/vnd.swift.registry.v1+json"
VERIFY_AUTH=""
[ "$E2E_USE_HTTPS" = true ] && VERIFY_AUTH="-u ${E2E_USER}:${E2E_PASS}"
PUBLISH_OPTS="--url $REGISTRY_URL"
[ "$E2E_USE_HTTPS" = false ] && PUBLISH_OPTS="$PUBLISH_OPTS --allow-insecure-http"

# Publish SamplePackage 1.0.0 and 1.1.0
step "SamplePackage:"
for VERSION in 1.0.0 1.1.0; do
	PACKAGE_NAME="SamplePackage"
	PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
	info "Preparing $PACKAGE_ID $VERSION..."
	cd "$ROOT_DIR/testdata/e2e/example.SamplePackage"
	swift package dump-package > Package.json 2>/dev/null || true
	swift package-registry publish "$PACKAGE_ID" "$VERSION" $PUBLISH_OPTS && ok "Published $PACKAGE_ID $VERSION"
done

# Publish UtilsPackage 1.0.0 and 1.1.0
step "UtilsPackage:"
for VERSION in 1.0.0 1.1.0; do
	PACKAGE_NAME="UtilsPackage"
	PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
	info "Preparing $PACKAGE_ID $VERSION..."
	cd "$ROOT_DIR/testdata/e2e/example.UtilsPackage"
	swift package dump-package > Package.json 2>/dev/null || true
	swift package-registry publish "$PACKAGE_ID" "$VERSION" $PUBLISH_OPTS && ok "Published $PACKAGE_ID $VERSION"
done

# Verify package metadata and manifest for SamplePackage 1.0.0
section "Verification"

PACKAGE_NAME="SamplePackage"
VERSION="1.0.0"
PACKAGE_ID="${SCOPE}.${PACKAGE_NAME}"
info "Verifying $PACKAGE_ID $VERSION metadata..."
INFO_JSON=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}")
if ! echo "$INFO_JSON" | grep -q '"metadata"'; then
	printf "${C_RED}Package info response missing metadata.${C_RESET}\n"
	exit 1
fi
if ! echo "$INFO_JSON" | grep -q '"description"'; then
	printf "${C_RED}Package info metadata missing description.${C_RESET}\n"
	exit 1
fi
ok "Package metadata"

info "Verifying alternative manifest (Package@swift-5.8) for $PACKAGE_ID..."
ACCEPT_SWIFT="Accept: application/vnd.swift.registry.v1+swift"
MANIFEST_58=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_SWIFT" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}/${VERSION}/Package.swift?swift-version=5.8")
if ! echo "$MANIFEST_58" | grep -q "swift-tools-version:5.8"; then
	printf "${C_RED}Package@swift-5.8 manifest not found or wrong version.${C_RESET}\n"
	exit 1
fi
ok "Alternative manifest (swift-5.8)"

info "Verifying list releases (GET /{scope}/{name})..."
LIST_RESP=$(curl $CURL_OPTS $VERIFY_AUTH -D - -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}")
if ! echo "$LIST_RESP" | grep -q '"1.0.0"'; then
	printf "${C_RED}List response missing version 1.0.0.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_RESP" | grep -q '"1.1.0"'; then
	printf "${C_RED}List response missing version 1.1.0.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_RESP" | grep -q 'rel="latest-version"'; then
	printf "${C_RED}List response missing latest-version Link header.${C_RESET}\n"
	exit 1
fi
ok "List releases (1.0.0, 1.1.0, latest-version link)"

info "Verifying list pagination (next/prev/first/last per spec 4.1)..."
LIST_PAGE1=$(curl $CURL_OPTS $VERIFY_AUTH -D - -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}?page=1")
if ! echo "$LIST_PAGE1" | grep -q '"1.1.0"'; then
	printf "${C_RED}Page 1 should return 1.1.0 (highest precedence first), got: $(echo "$LIST_PAGE1" | grep -o '"releases":{[^}]*}' || true)${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE1" | grep -q 'rel="first"'; then
	printf "${C_RED}Page 1 missing first link.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE1" | grep -q 'rel="next"'; then
	printf "${C_RED}Page 1 missing next link.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE1" | grep -q 'rel="last"'; then
	printf "${C_RED}Page 1 missing last link.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE1" | grep -q 'rel="prev"'; then
	: # prev is optional on page 1
fi
LIST_PAGE2=$(curl $CURL_OPTS $VERIFY_AUTH -D - -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/${PACKAGE_NAME}?page=2")
if ! echo "$LIST_PAGE2" | grep -q '"1.0.0"'; then
	printf "${C_RED}Page 2 should return 1.0.0, got: $(echo "$LIST_PAGE2" | grep -o '"releases":{[^}]*}' || true)${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE2" | grep -q 'rel="prev"'; then
	printf "${C_RED}Page 2 missing prev link.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE2" | grep -q 'rel="first"'; then
	printf "${C_RED}Page 2 missing first link.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_PAGE2" | grep -q 'rel="last"'; then
	printf "${C_RED}Page 2 missing last link.${C_RESET}\n"
	exit 1
fi
ok "Pagination and Link headers (first, prev, next, last)"

info "Verifying list for UtilsPackage..."
LIST_UTILS=$(curl $CURL_OPTS $VERIFY_AUTH -H "$ACCEPT_JSON" "${REGISTRY_URL}/${SCOPE}/UtilsPackage")
if ! echo "$LIST_UTILS" | grep -q '"1.0.0"'; then
	printf "${C_RED}UtilsPackage list missing 1.0.0.${C_RESET}\n"
	exit 1
fi
if ! echo "$LIST_UTILS" | grep -q '"1.1.0"'; then
	printf "${C_RED}UtilsPackage list missing 1.1.0.${C_RESET}\n"
	exit 1
fi
ok "UtilsPackage list"

info "Verifying package collection (global)..."
COLLECTION_GLOBAL=$(curl $CURL_OPTS $VERIFY_AUTH -H "Accept: application/json" "${REGISTRY_URL}/collection")
if ! echo "$COLLECTION_GLOBAL" | grep -q '"formatVersion"'; then
	printf "${C_RED}Global collection response missing formatVersion.${C_RESET}\n"
	exit 1
fi
if ! echo "$COLLECTION_GLOBAL" | grep -q '"packages"'; then
	printf "${C_RED}Global collection response missing packages array.${C_RESET}\n"
	exit 1
fi
for PKG in example.SamplePackage example.UtilsPackage; do
	if ! echo "$COLLECTION_GLOBAL" | grep -q "\"${PKG}\""; then
		printf "${C_RED}Global collection does not contain ${PKG}.${C_RESET}\n"
		exit 1
	fi
done
if ! echo "$COLLECTION_GLOBAL" | grep -q '"generatedBy"'; then
	printf "${C_RED}Global collection response missing generatedBy.${C_RESET}\n"
	exit 1
fi
ok "Global collection (contains both packages)"

info "Verifying package collection (scope ${SCOPE})..."
COLLECTION_SCOPE=$(curl $CURL_OPTS $VERIFY_AUTH -H "Accept: application/json" "${REGISTRY_URL}/collection/${SCOPE}")
for PKG_ID in example.SamplePackage example.UtilsPackage; do
	if ! echo "$COLLECTION_SCOPE" | grep -q "\"${PKG_ID}\""; then
		printf "${C_RED}Scope collection /collection/${SCOPE} does not contain ${PKG_ID}.${C_RESET}\n"
		exit 1
	fi
done
for VER in 1.0.0 1.1.0; do
	if ! echo "$COLLECTION_SCOPE" | grep -q "\"${VER}\""; then
		printf "${C_RED}Scope collection does not contain version ${VER}.${C_RESET}\n"
		exit 1
	fi
done
ok "Scope collection (both packages, multiple versions)"

info "Verifying package collection (non-existent scope returns 404)..."
COLLECTION_404=$(curl $CURL_OPTS $VERIFY_AUTH -w "%{http_code}" -o /dev/null -H "Accept: application/json" "${REGISTRY_URL}/collection/nonexistentscope123")
if [ "$COLLECTION_404" != "404" ]; then
	printf "${C_RED}Expected 404 for non-existent scope, got ${COLLECTION_404}.${C_RESET}\n"
	exit 1
fi
ok "404 for non-existent scope"

info "Verifying Swift package-collection CLI (add, list, describe)..."
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
	printf "${C_RED}swift package-collection add failed.${C_RESET}\n"
	exit 1
}
ok "Collection add"
if ! swift package-collection list 2>/dev/null | grep -q "All Packages"; then
	rm -f "$COLLECTION_FILE"
	swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
	printf "${C_RED}swift package-collection list: collection not found.${C_RESET}\n"
	exit 1
fi
ok "Collection list"
if ! swift package-collection describe "$COLLECTION_ADD_URL" 2>/dev/null | grep -qi "example"; then
	rm -f "$COLLECTION_FILE"
	swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
	printf "${C_RED}swift package-collection describe: package not found in collection.${C_RESET}\n"
	exit 1
fi
ok "Collection describe"
swift package-collection remove "$COLLECTION_ADD_URL" 2>/dev/null || true
rm -f "$COLLECTION_FILE"

# Verification summary table
echo ""
printf "  ${C_DIM}┌────────────────────────────────┬────────┐${C_RESET}\n"
printf "  ${C_DIM}│ %-30s │ %-6s │${C_RESET}\n" "Verification" "Status"
printf "  ${C_DIM}├────────────────────────────────┼────────┤${C_RESET}\n"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Package metadata"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Alt. manifest (swift-5.8)"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "List releases"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Pagination"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "UtilsPackage list"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Global collection"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Scope collection"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "404 non-existent scope"
printf "  ${C_DIM}│ %-30s │ ${C_GREEN}✓${C_RESET}      │${C_RESET}\n" "Package-collection CLI"
printf "  ${C_DIM}└────────────────────────────────┴────────┘${C_RESET}\n"
echo ""

section "Consumer"
info "Configuring consumer to use registry and resolving..."
cd "$ROOT_DIR/testdata/e2e/Consumer"
if [ "$E2E_USE_HTTPS" = true ]; then
	swift package-registry set "$REGISTRY_URL"
else
	swift package-registry set "$REGISTRY_URL" --allow-insecure-http
fi
RESOLVE_OUT=$(swift package resolve 2>&1)
echo "$RESOLVE_OUT"
echo "$RESOLVE_OUT" | grep -qi "warning" && E2E_HAD_WARNING=1

if [ ! -f Package.resolved ]; then
	printf "${C_RED}Package.resolved was not created; resolve may have failed.${C_RESET}\n"
	exit 1
fi
for PKG in example.SamplePackage example.UtilsPackage; do
	if ! grep -q "$PKG" Package.resolved; then
		printf "${C_RED}Package.resolved does not contain $PKG.${C_RESET}\n"
		exit 1
	fi
done
ok "Consumer resolve (both packages)"

info "Building and running Consumer..."
swift build
OUTPUT=$(swift run Consumer 2>&1)
echo "$OUTPUT" | grep -qi "warning" && E2E_HAD_WARNING=1
if ! echo "$OUTPUT" | grep -q "Resolved SamplePackage"; then
	printf "${C_RED}Consumer output missing SamplePackage: $OUTPUT${C_RESET}\n"
	exit 1
fi
if ! echo "$OUTPUT" | grep -q "Resolved UtilsPackage"; then
	printf "${C_RED}Consumer output missing UtilsPackage: $OUTPUT${C_RESET}\n"
	exit 1
fi
ok "Consumer build and run"

# Final summary
echo ""
PASS_MSG="  E2E Swift Publish and Resolve: PASSED"
printf "${C_BOLD}${C_GREEN}╭─────────────────────────────────────────────────────────────────╮${C_RESET}\n"
printf "${C_BOLD}${C_GREEN}│${C_RESET} ${C_GREEN}✓ %-62s${C_RESET}${C_BOLD}${C_GREEN}│${C_RESET}\n" "$PASS_MSG"
printf "${C_BOLD}${C_GREEN}╰─────────────────────────────────────────────────────────────────╯${C_RESET}\n"
echo ""
