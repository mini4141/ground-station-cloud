#!/usr/bin/env bash
# Generates a local CA, the broker certificate and per-identity client certificates into .local/certs/ (gitignored).
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/.."
DIR=.local/certs
mkdir -p "$DIR"
if [ -f "$DIR/ca.crt" ] && [ -f "$DIR/tokyo.crt" ] && [ "${FORCE:-}" != "1" ]; then
  echo "certs already present in $DIR (FORCE=1 to regenerate)"; exit 0
fi
rm -f "$DIR"/*.crt "$DIR"/*.key "$DIR"/*.csr "$DIR"/*.srl "$DIR"/*.ext

openssl req -x509 -newkey rsa:2048 -nodes -days 365 -keyout "$DIR/ca.key" -out "$DIR/ca.crt" -subj "/CN=groundstation-local-ca" 2>/dev/null

issue() { # name, CN, extKeyUsage, SAN
  local name=$1 cn=$2 eku=$3 san=$4
  openssl req -newkey rsa:2048 -nodes -keyout "$DIR/$name.key" -out "$DIR/$name.csr" -subj "/CN=$cn" 2>/dev/null
  printf "basicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=%s\n%s\n" "$eku" "$san" > "$DIR/$name.ext"
  openssl x509 -req -in "$DIR/$name.csr" -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" -CAcreateserial -days 365 -out "$DIR/$name.crt" -extfile "$DIR/$name.ext" 2>/dev/null
  rm -f "$DIR/$name.csr" "$DIR/$name.ext"
}

issue server mosquitto serverAuth "subjectAltName=DNS:mosquitto,DNS:localhost,IP:127.0.0.1"
for id in cloud tokyo svalbard santiago; do issue "$id" "$id" clientAuth "subjectAltName=DNS:$id"; done
chmod 644 "$DIR"/*.key # containers run as non-root users; these are throwaway local dev keys
echo "generated CA, broker and client certs (cloud tokyo svalbard santiago) in $DIR"
