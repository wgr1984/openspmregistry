#!/usr/bin/env bash
# Generate self-signed certs for E2E HTTPS testing.
# Cert includes SAN for 127.0.0.1 and localhost.
# Run from repo root. Certs are written to testdata/e2e/certs/

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CERTS_DIR="$ROOT_DIR/testdata/e2e/certs"
CN="OpenSPMRegistry E2E Test"

mkdir -p "$CERTS_DIR"
cd "$CERTS_DIR"

openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes \
  -subj "/CN=$CN" \
  -addext "subjectAltName=DNS:127.0.0.1,IP:127.0.0.1,DNS:localhost,IP:::1"

chmod 600 server.key
echo "Generated $CERTS_DIR/server.crt and server.key"
