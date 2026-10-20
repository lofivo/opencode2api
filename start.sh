#!/bin/sh
# Start opencode2api in the background. Run with --help for supported options.
set -eu

root=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$root"

usage() {
    cat <<'EOF'
Usage: ./start.sh [opencode2api options...]

Start the built gateway in the background using CONFIG_PATH.

Environment:
  CONFIG_PATH   Configuration file (default: ./config.json).
  BINARY        Gateway binary (default: ./opencode2api).
  STATE_DIR     Runtime directory (default: ./.local).
  PID_FILE      PID file (default: $STATE_DIR/opencode2api.pid).
  LOG_FILE      Append-only log (default: $STATE_DIR/opencode2api.log).

Extra arguments are passed to opencode2api after -config, for example:
  ./start.sh -listen 0.0.0.0:8080
EOF
}

case ${1:-} in
    -h|--help)
        usage
        exit 0
        ;;
esac

abspath() {
    case $1 in
        /*) printf '%s\n' "$1" ;;
        *) printf '%s\n' "$root/$1" ;;
    esac
}

state_dir=$(abspath "${STATE_DIR:-$root/.local}")
config_path=$(abspath "${CONFIG_PATH:-$root/config.json}")
binary=$(abspath "${BINARY:-$root/opencode2api}")
pid_file=$(abspath "${PID_FILE:-$state_dir/opencode2api.pid}")
log_file=$(abspath "${LOG_FILE:-$state_dir/opencode2api.log}")

if [ ! -x "$binary" ]; then
    printf 'start.sh: binary not found or not executable: %s\nRun ./build.sh first, or set BINARY.\n' "$binary" >&2
    exit 1
fi

if [ ! -f "$config_path" ]; then
    printf 'start.sh: configuration not found: %s\nCreate it first:\n  cp config.example.json %s\nThen set server_keys and either upstream keys or anonymous mode.\n' "$config_path" "$config_path" >&2
    exit 1
fi

umask 077
mkdir -p "$state_dir" "$(dirname "$pid_file")" "$(dirname "$log_file")"

if [ -f "$pid_file" ]; then
    pid=$(sed -n '1p' "$pid_file" 2>/dev/null || true)
    case $pid in
        ''|*[!0-9]*|0*|1)
            printf 'start.sh: removing invalid PID file %s\n' "$pid_file" >&2
            rm -f "$pid_file"
            ;;
        *)
            if kill -0 "$pid" 2>/dev/null; then
                printf 'opencode2api is already running (pid %s)\n' "$pid"
                exit 0
            fi
            printf 'start.sh: removing stale PID file %s\n' "$pid_file" >&2
            rm -f "$pid_file"
            ;;
    esac
fi

printf '\n[%s] starting %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$binary" >>"$log_file"
nohup "$binary" -config "$config_path" "$@" </dev/null >>"$log_file" 2>&1 &
pid=$!
printf '%s\n' "$pid" >"$pid_file"
sleep 1

if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$pid_file"
    printf 'start.sh: process exited during startup; last log lines:\n' >&2
    tail -n 30 "$log_file" >&2 || true
    exit 1
fi

printf 'opencode2api started (pid %s)\n' "$pid"
printf '  config: %s\n' "$config_path"
printf '  log:    %s\n' "$log_file"
printf '  pid:    %s\n' "$pid_file"
printf 'Run ./stop.sh to stop it.\n'
