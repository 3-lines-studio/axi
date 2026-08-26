#!/bin/sh
set -eu

ecosystem="$(cd "$(dirname "$0")/../.." && pwd)"
address="${AXI_WEB_ADDRESS:-127.0.0.1:7777}"
bin_dir="$HOME/.local/bin"
log_dir="$HOME/.local/share/axi/logs"
export PATH="$HOME/.local/share/mise/shims:$PATH"

log() {
    printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$log_dir/dev-update.log"
}

mkdir -p "$log_dir"
if [ "${1:-}" != "--worker" ]; then
    setsid "$0" --worker </dev/null >> "$log_dir/dev-update.log" 2>&1 &
    echo "Axi update queued; it will restart after active runs finish."
    exit 0
fi

staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT
log "dev update started"

npm --prefix "$ecosystem/axi/frontend" run build >> "$log_dir/dev-update.log" 2>&1
CGO_ENABLED=0 go build -C "$ecosystem/axi" -tags headless -trimpath -o "$staging/axi" . >> "$log_dir/dev-update.log" 2>&1
CGO_ENABLED=0 go build -C "$ecosystem/axis" -trimpath -o "$staging/axis" . >> "$log_dir/dev-update.log" 2>&1
(cd "$ecosystem/ax" && cargo +nightly build --release) >> "$log_dir/dev-update.log" 2>&1
cp "$ecosystem/ax/target/release/ax" "$staging/ax"
log "build ok"

for name in axi axis ax; do
    cp "$bin_dir/$name" "$staging/$name.previous"
done

for _ in $(seq 1 120); do
    runs="$(curl -fsS "http://$address/api/runs" 2>/dev/null | grep -c '"status":"running"' || true)"
    if [ "$runs" = "0" ]; then
        break
    fi
    sleep 5
done
runs="$(curl -fsS "http://$address/api/runs" 2>/dev/null | grep -c '"status":"running"' || true)"
if [ "$runs" != "0" ]; then
    log "runs still active after timeout"
    exit 1
fi
log "runs drained"

if curl -fsS -m 1 "http://$address/health" >/dev/null 2>&1; then
    curl -fsS -X POST "http://$address/api/local/quit" >/dev/null
    for _ in $(seq 1 100); do
        if ! curl -fsS -m 1 "http://$address/health" >/dev/null 2>&1; then
            break
        fi
        sleep 0.1
    done
    if curl -fsS -m 1 "http://$address/health" >/dev/null 2>&1; then
        log "daemon did not stop"
        exit 1
    fi
fi
log "daemon stopped"

for name in axi axis ax; do
    install -m755 "$staging/$name" "$bin_dir/.$name.new"
    mv "$bin_dir/.$name.new" "$bin_dir/$name"
done
log "binaries installed"

setsid "$bin_dir/axi" web </dev/null >> "$log_dir/dev-update.log" 2>&1 &
for _ in $(seq 1 100); do
    if curl -fsS -m 1 "http://$address/health" >/dev/null 2>&1; then
        log "daemon healthy"
        exit 0
    fi
    sleep 0.1
done

log "restart failed, rolling back"
for name in axi axis ax; do
    install -m755 "$staging/$name.previous" "$bin_dir/.$name.new"
    mv "$bin_dir/.$name.new" "$bin_dir/$name"
done
setsid "$bin_dir/axi" web </dev/null >> "$log_dir/dev-update.log" 2>&1 &
exit 1
