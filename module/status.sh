#!/system/bin/sh

MODDIR=${0%/*}
BIN="$MODDIR/bin/limbus-proxy"
DATA=/data/adb/limbus-localization
PID="$DATA/limbus-proxy.pid"

proxy=已停止
if [ -s "$PID" ]; then
    process_id=$(cat "$PID")
    if [ "$(readlink -f "/proc/$process_id/exe" 2>/dev/null)" = "$BIN" ]; then
        proxy=运行中
    fi
fi

resource=$(sed -n '1p' "$DATA/current-version" 2>/dev/null)
[ -n "$resource" ] || resource=未安装

ca=未挂载
sdk=$(getprop ro.build.version.sdk)
case "$sdk" in
    ''|*[!0-9]*) sdk=0 ;;
esac
for cert in "$DATA/system-ca"/*.0; do
    [ -f "$cert" ] || continue
    cert_name=${cert##*/}
    if [ "$sdk" -ge 34 ] && [ -f "/apex/com.android.conscrypt/cacerts/$cert_name" ]; then
        ca=已挂载
        break
    fi
    if [ "$sdk" -lt 34 ] && [ -f "/system/etc/security/cacerts/$cert_name" ]; then
        ca=已挂载
        break
    fi
done

route=未映射
if grep -q '^127\.0\.0\.1[[:space:]][[:space:]]*downloadcommon\.limbuscompanycdn\.org$' /system/etc/hosts 2>/dev/null; then
    route=已映射
fi

if [ "$proxy" = "运行中" ] && [ "$ca" = "已挂载" ] && [ "$route" = "已映射" ]; then
    overall=正常
else
    overall=异常
fi

description="状态:$overall｜代理:$proxy｜汉化:$resource｜CA:$ca｜域名:$route"
sed -i "s#^description=.*#description=$description#" "$MODDIR/module.prop"
