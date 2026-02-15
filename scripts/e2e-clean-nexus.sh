#!/usr/bin/env bash
# Delete example scope E2E package components (SamplePackage, UtilsPackage) from Nexus private repo.
# Used by E2E test to ensure clean state before publish.
set -e

NEXUS_URL="${NEXUS_URL:-http://localhost:8081}"
REPO="${MAVEN_REPO_NAME:-private}"
USER="${MAVEN_REPO_USERNAME:-admin}"
PASS="${MAVEN_REPO_PASSWORD:-admin123}"

# Packages to clean: scope.name format; we search by group=example and name=<PackageName>
E2E_PACKAGES="${E2E_PACKAGES:-SamplePackage UtilsPackage}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS_FILE="$SCRIPT_DIR/../.nexus-test-password"
if [ -f "$PASS_FILE" ]; then
	PASS=$(cat "$PASS_FILE")
fi

AUTH=$(echo -n "$USER:$PASS" | base64)
TOTAL_DELETED=0

for PKG_NAME in $E2E_PACKAGES; do
	BASE_URL="$NEXUS_URL/service/rest/v1/search?repository=$REPO&group=example&name=$PKG_NAME"
	DELETED=0
	TOKEN=""

	while true; do
		if [ -n "$TOKEN" ]; then
			URL="$BASE_URL&continuationToken=$TOKEN"
		else
			URL="$BASE_URL"
		fi
		RESP=$(curl -s -w "\n%{http_code}" -H "Authorization: Basic $AUTH" "$URL" 2>/dev/null) || true
		CODE=$(echo "$RESP" | tail -n 1)
		BODY=$(echo "$RESP" | sed '$d')
		if [ "$CODE" = "404" ] || [ -z "$BODY" ]; then
			break
		fi
		if [ "$CODE" != "200" ]; then
			echo "Nexus search failed (HTTP $CODE)" >&2
			exit 1
		fi

		# Extract component IDs (format: "id":"<value>")
		ids=$(echo "$BODY" | grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/')
		for cid in $ids; do
			[ -z "$cid" ] && continue
			if curl -sf -X DELETE -H "Authorization: Basic $AUTH" "$NEXUS_URL/service/rest/v1/components/$cid" >/dev/null 2>&1; then
				DELETED=$((DELETED + 1))
			fi
		done

		TOKEN=$(echo "$BODY" | grep -oE '"continuationToken"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/' || true)
		[ -z "$TOKEN" ] && break
	done

	TOTAL_DELETED=$((TOTAL_DELETED + DELETED))
	[ "$DELETED" -gt 0 ] && echo "Cleaned $DELETED example.$PKG_NAME component(s) from Nexus"
done

if [ "$TOTAL_DELETED" -gt 0 ]; then
	echo "Total cleaned: $TOTAL_DELETED component(s)"
fi
