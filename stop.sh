#!/bin/sh
# Stop the process started by start.sh. Run with --help for supported options.
set -eu

root=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$root"

usage() {
    cat <<'EOF'
Usage: ./stop.sh

Stop the process recorded in PID_FILE. SIGTERM is sent first; if the process
is still alive after STOP_TIMEOUT seconds, SIGKILL is sent.

Environment:
  STATE_DIR     Runtime directory (default: ./.local).
  PID_FILE      PID file (default: $STATE_DIR/opencode2api.pid).
  STOP_TIMEOUT  Seconds to wait for a graceful exit (default: 20).
EOF
}

case ${1:-} in
    -h|--help)
        usage
        exit 0
        ;;
    "")
        ;;
    *)
        printf 'unknown argument: %s\n' "$1" >&2
        usage >&2
        exit 2
        ;;
esac

abspath() {
    case $1 in
        /*) printf '%s\n' "$1" ;;
        *) printf '%s\n' "$root/$1" ;;
    esac
}

state_dir=$(abspath "${STATE_DIR:-$root/.local}")
pid_file=$(abspath "${PID_FILE:-$state_dir/opencode2api.pid}")
stop_timeout=${STOP_TIMEOUT:-20}

case $stop_timeout in
    ''|*[!0-9]*)
        printf 'stop.sh: STOP_TIMEOUT must be a non-negative integer\n' >&2
        exit 2
        ;;
esac

if [ ! -f "$pid_file" ]; then
    printf 'opencode2api is not running (no PID file: %s)\n' "$pid_file"
    exit 0
fi

pid=$(sed -n '1p' "$pid_file" 2>/dev/null || true)
case $pid in
    ''|*[!0-9]*|0*|1)
        printf 'stop.sh: removing invalid PID file %s\n' "$pid_file" >&2
        rm -f "$pid_file"
        exit 0
        ;;
esac

if ! kill -0 "$pid" 2>/dev/null; then
    printf 'opencode2api is not running (stale pid %s); removing PID file\n' "$pid" >&2
    rm -f "$pid_file"
    exit 0
fi

printf 'stopping opencode2api (pid %s)...\n' "$pid"
kill -TERM "$pid" 2>/dev/null || true

elapsed=0
while kill -0 "$pid" 2>/dev/null; do
    if [ "$elapsed" -ge "$stop_timeout" ]; then
        printf 'stop.sh: pid %s did not exit after %ss; sending SIGKILL\n' "$pid" "$stop_timeout" >&2
        kill -KILL "$pid" 2>/dev/null || true
        sleep 1
        break
    fi
    elapsed=$((elapsed + 1))
    sleep 1
done

if kill -0 "$pid" 2>/dev/null; then
    printf 'stop.sh: failed to stop pid %s\n' "$pid" >&2
    exit 1
fi

rm -f "$pid_file"
printf 'opencode2api stopped\n'
