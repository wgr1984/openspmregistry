#!/usr/bin/env bash
# After Nexus is up: get initial admin password, create Maven repo "private" via Script API, set admin password to admin123.
# Idempotent: safe to run multiple times (repo may already exist; password may already be set).
set -e

NEXUS_URL="${NEXUS_URL:-http://localhost:8081}"
NEXUS_CONTAINER="${NEXUS_CONTAINER:-nexus-test}"
REPO_KEY="${NEXUS_REPO_KEY:-private}"
TARGET_PASSWORD="${NEXUS_TARGET_PASSWORD:-admin123}"

echo "Bootstrapping Nexus at ${NEXUS_URL}..."

# Get initial admin password (Nexus 3.17+ stores it in the container)
ADMIN_PASSWORD=""
if docker exec "${NEXUS_CONTAINER}" test -f /nexus-data/admin.password 2>/dev/null; then
  ADMIN_PASSWORD=$(docker exec "${NEXUS_CONTAINER}" cat /nexus-data/admin.password 2>/dev/null | tr -d '\r\n' || true)
fi
if [ -z "$ADMIN_PASSWORD" ]; then
  echo "No admin.password file found; assuming already configured, using target password for API calls."
  ADMIN_PASSWORD="${TARGET_PASSWORD}"
fi

# Create Maven hosted repo via Script API (Nexus 3.21.1 and earlier allow script creation by default)
SCRIPT_NAME="maven-hosted-${REPO_KEY}"
echo "Creating Maven repository: ${REPO_KEY}"
resp=$(curl -s -w "\n%{http_code}" -u "admin:${ADMIN_PASSWORD}" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${SCRIPT_NAME}\",\"type\":\"groovy\",\"content\":\"repository.createMavenHosted('${REPO_KEY}')\"}" \
  "${NEXUS_URL}/service/rest/v1/script")
code=$(echo "$resp" | tail -n 1)
body=$(echo "$resp" | sed '$d')

case "$code" in
  200|201|204) echo "Script uploaded." ;;
  400) echo "Script already exists, continuing." ;;
  500)
    if echo "$body" | grep -qi "duplicated\|script_name_idx\|DuplicatedException"; then
      echo "Script already exists (Nexus returned 500 for duplicate name), continuing."
    else
      echo "Failed to upload script (HTTP 500): $body" >&2
      exit 1
    fi
    ;;
  *)
    echo "Failed to upload script (HTTP ${code}): $body" >&2
    exit 1
    ;;
esac

# Run the script (idempotent: repo may already exist)
run_resp=$(curl -s -w "\n%{http_code}" -u "admin:${ADMIN_PASSWORD}" \
  -X POST \
  -H "Content-Type: text/plain" \
  "${NEXUS_URL}/service/rest/v1/script/${SCRIPT_NAME}/run")
run_code=$(echo "$run_resp" | tail -n 1)
run_body=$(echo "$run_resp" | sed '$d')

if [ "$run_code" = "200" ]; then
  echo "Repository ${REPO_KEY} created or already exists."
elif echo "$run_body" | grep -qi "already exists\|Conflict\|409"; then
  echo "Repository ${REPO_KEY} already exists."
else
  echo "Script run response (HTTP ${run_code}): $run_body"
  if [ "$run_code" != "200" ]; then
    echo "Warning: script run returned ${run_code}; continuing (repo may already exist)." >&2
  fi
fi

# Configure repo: disable strict content type validation (for application/pgp-signature) and set writePolicy ALLOW (multiple packages)
CONFIGURE_REPO_SCRIPT="configure-repo-${REPO_KEY}"
GROOVY_CONFIGURE_REPO="def repo = repository.repositoryManager.get('${REPO_KEY}'); if (repo != null) { def config = repo.configuration; def storage = config.attributes('storage'); storage.set('strictContentTypeValidation', false); storage.set('writePolicy', 'ALLOW'); repository.repositoryManager.update(config); return 'ok'; }; return 'repo not found';"
resp2=$(curl -s -w "\\n%{http_code}" -u "admin:${ADMIN_PASSWORD}" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${CONFIGURE_REPO_SCRIPT}\",\"type\":\"groovy\",\"content\":\"$(printf '%s' "$GROOVY_CONFIGURE_REPO" | sed 's/\\/\\\\/g; s/"/\\"/g')\"}" \
  "${NEXUS_URL}/service/rest/v1/script")
code2=$(echo "$resp2" | tail -n 1)
body2=$(echo "$resp2" | sed '$d')
case "$code2" in
  200|201|204) echo "Script ${CONFIGURE_REPO_SCRIPT} uploaded." ;;
  400) echo "Script ${CONFIGURE_REPO_SCRIPT} already exists, continuing." ;;
  500)
    if echo "$body2" | grep -qi "duplicated\|script_name_idx\|DuplicatedException"; then
      echo "Script ${CONFIGURE_REPO_SCRIPT} already exists, continuing."
    else
      echo "Warning: upload script ${CONFIGURE_REPO_SCRIPT} (HTTP 500): $body2" >&2
    fi
    ;;
  *) echo "Warning: upload script ${CONFIGURE_REPO_SCRIPT} (HTTP ${code2})." >&2 ;;
esac
run2=$(curl -s -w "\\n%{http_code}" -u "admin:${ADMIN_PASSWORD}" \
  -X POST \
  -H "Content-Type: text/plain" \
  "${NEXUS_URL}/service/rest/v1/script/${CONFIGURE_REPO_SCRIPT}/run")
run2_code=$(echo "$run2" | tail -n 1)
run2_body=$(echo "$run2" | sed '$d')
if [ "$run2_code" = "200" ]; then
  echo "Repo ${REPO_KEY} configured (strictContentTypeValidation=false, writePolicy=ALLOW)."
else
  echo "Warning: configure repo script returned ${run2_code}: $run2_body" >&2
fi

# Set admin password to admin123 so integration tests can use fixed credentials
EFFECTIVE_PASSWORD="${TARGET_PASSWORD}"
if [ "$ADMIN_PASSWORD" != "$TARGET_PASSWORD" ]; then
  echo "Setting admin password to ${TARGET_PASSWORD}..."
  if curl -sf -u "admin:${ADMIN_PASSWORD}" \
    -X PUT \
    -H "Content-Type: text/plain" \
    -d "${TARGET_PASSWORD}" \
    "${NEXUS_URL}/service/rest/v1/security/users/admin/change-password" > /dev/null; then
    echo "Admin password updated."
    EFFECTIVE_PASSWORD="${TARGET_PASSWORD}"
  else
    echo "Warning: could not change admin password; tests will use initial password from .nexus-test-password" >&2
    EFFECTIVE_PASSWORD="${ADMIN_PASSWORD}"
  fi
fi

# Write password for Makefile so integration tests can authenticate (change-password may fail on first run)
if [ -n "${NEXUS_TEST_PASSWORD_FILE:-}" ]; then
  printf '%s' "$EFFECTIVE_PASSWORD" > "$NEXUS_TEST_PASSWORD_FILE"
  echo "Wrote effective password to ${NEXUS_TEST_PASSWORD_FILE}"
fi

echo "Nexus bootstrap done."
