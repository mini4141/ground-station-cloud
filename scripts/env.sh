# Sourced by the other scripts: loads .env (or .env.example) and makes paths absolute.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
set -a
# shellcheck disable=SC1090
source "$ROOT/.env.example"
[ -f "$ROOT/.env" ] && source "$ROOT/.env"
set +a
cd "$ROOT"
mkdir -p .local/bin .local/logs .local/run .local/uploads
export MSYS_NO_PATHCONV=1
EXE=""; case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=".exe";; esac
export EXE

# start_bg <name> <cmd...>: run in background with a log and pid file
start_bg() {
  local name=$1; shift
  nohup "$@" > ".local/logs/$name.log" 2>&1 &
  echo $! > ".local/run/$name.pid"
}
stop_bg() {
  local f=".local/run/$1.pid"
  [ -f "$f" ] || return 0
  kill -f "$(cat "$f")" 2>/dev/null || kill "$(cat "$f")" 2>/dev/null || true
  rm -f "$f"
}
wait_http() { # url, seconds
  local i
  for i in $(seq 1 "${2:-60}"); do curl -fsS -m 3 "$1" >/dev/null 2>&1 && return 0; sleep 1; done
  echo "timeout waiting for $1" >&2; return 1
}
