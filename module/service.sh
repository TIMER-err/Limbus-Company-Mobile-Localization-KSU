#!/system/bin/sh

MODDIR=${0%/*}
BIN="$MODDIR/bin/limbus-proxy"
DATA=/data/adb/limbus-localization
PID="$DATA/limbus-proxy.pid"
LOG="$DATA/logs/service.log"

mkdir -p "$DATA/logs"

is_running() {
    [ -s "$PID" ] || return 1
    process_id=$(cat "$PID")
    case "$process_id" in
        ''|*[!0-9]*) return 1 ;;
    esac
    [ "$(readlink -f "/proc/$process_id/exe" 2>/dev/null)" = "$BIN" ]
}

is_running && exit 0
rm -f "$PID"

"$BIN" serve --data "$DATA" >>"$LOG" 2>&1 &
echo $! > "$PID"
sleep 1
if ! is_running; then
    rm -f "$PID"
    echo "limbus-proxy 启动失败，请查看 $LOG" >> "$LOG"
    exit 1
fi
