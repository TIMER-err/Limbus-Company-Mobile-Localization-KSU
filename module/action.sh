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
update_result=$("$BIN" update --data "$DATA" 2>&1)
update_status=$?
echo "$update_result"
if [ "$update_status" -ne 0 ]; then
    echo ""
    echo "更新检查失败，请稍后重试。"
    "$MODDIR/status.sh"
    exit "$update_status"
fi

if ! is_running; then
    "$MODDIR/service.sh"
fi

echo ""
"$BIN" status --data "$DATA"
if is_running; then
    echo "服务状态：运行中（PID $(cat "$PID")）"
else
    echo "服务状态：启动失败，请查看 $DATA/logs/service.log"
    "$MODDIR/status.sh"
    exit 1
fi

"$MODDIR/status.sh"
echo ""
echo "操作完成。"
