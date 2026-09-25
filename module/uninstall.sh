#!/system/bin/sh

MODDIR=${0%/*}
DATA=/data/adb/limbus-localization
BIN="$MODDIR/bin/limbus-proxy"
PID="$DATA/limbus-proxy.pid"

if [ -s "$PID" ]; then
    process_id=$(cat "$PID")
    if [ "$(readlink -f "/proc/$process_id/exe" 2>/dev/null)" = "$BIN" ]; then
        kill "$process_id" 2>/dev/null || true
    fi
fi
rm -rf "$DATA"
