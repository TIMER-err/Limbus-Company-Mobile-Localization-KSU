#!/system/bin/sh

MODDIR=${0%/*}
BIN="$MODDIR/bin/limbus-proxy"
DATA=/data/adb/limbus-localization
PID="$DATA/limbus-proxy.pid"

proxy_icon=❌
if [ -s "$PID" ]; then
    process_id=$(cat "$PID")
    if [ "$(readlink -f "/proc/$process_id/exe" 2>/dev/null)" = "$BIN" ]; then
        proxy_icon=✅
    fi
fi

resource=$(sed -n '1p' "$DATA/current-version" 2>/dev/null)
[ -n "$resource" ] || resource=未安装

ca_icon=❌
sdk=$(getprop ro.build.version.sdk)
case "$sdk" in
    ''|*[!0-9]*) sdk=0 ;;
esac
for cert in "$DATA/system-ca"/*.0; do
    [ -f "$cert" ] || continue
    cert_name=${cert##*/}
    if [ "$sdk" -ge 34 ] && [ -f "/apex/com.android.conscrypt/cacerts/$cert_name" ]; then
        ca_icon=✅
        break
    fi
    if [ "$sdk" -lt 34 ] && [ -f "/system/etc/security/cacerts/$cert_name" ]; then
        ca_icon=✅
        break
    fi
done

route_icon=❌
if grep -q '^127\.0\.0\.1[[:space:]][[:space:]]*downloadcommon\.limbuscompanycdn\.org$' /system/etc/hosts 2>/dev/null; then
    route_icon=✅
fi

if [ "$proxy_icon" = "✅" ] && [ "$ca_icon" = "✅" ] && [ "$route_icon" = "✅" ]; then
    overall="✅ 运行正常"
else
    overall="⚠️ 状态异常"
fi

description="$overall｜代理$proxy_icon｜汉化:$resource｜CA$ca_icon｜域名$route_icon"
sed -i "s#^description=.*#description=$description#" "$MODDIR/module.prop"
