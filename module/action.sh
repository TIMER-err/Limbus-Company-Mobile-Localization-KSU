#!/system/bin/sh

MODDIR=${0%/*}
BIN="$MODDIR/bin/limbus-proxy"
DATA=/data/adb/limbus-localization
PID="$DATA/limbus-proxy.pid"

is_running() {
    [ -s "$PID" ] || return 1
    process_id=$(cat "$PID")
    case "$process_id" in
        ''|*[!0-9]*) return 1 ;;
    esac
    [ "$(readlink -f "/proc/$process_id/exe" 2>/dev/null)" = "$BIN" ]
}

echo "检查汉化更新……"
"$BIN" update --data "$DATA" || exit 1

if ! is_running; then
    "$MODDIR/service.sh"
fi

echo ""
"$BIN" status --data "$DATA"
if is_running; then
    echo "服务状态：运行中（PID $(cat "$PID")）"
else
    echo "服务状态：启动失败，请查看 $DATA/logs/service.log"
    exit 1
fi
