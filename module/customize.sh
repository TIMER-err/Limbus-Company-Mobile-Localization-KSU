#!/system/bin/sh

ui_print "*******************************"
ui_print " Limbus Company 移动端汉化"
ui_print "*******************************"

[ "$KSU" = "true" ] || abort "仅支持 KernelSU / SukiSU"
[ "$ARCH" = "arm64" ] || abort "当前仅支持 arm64 设备"

METAMODULE_DIR=$(readlink -f /data/adb/metamodule 2>/dev/null)
case "$METAMODULE_DIR" in
    /data/adb/modules/*) ;;
    *) abort "未检测到活动元模块；请先安装并启用 meta-overlayfs、Mountify 等元模块" ;;
esac
METAMODULE_PROP="$METAMODULE_DIR/module.prop"
[ -f "$METAMODULE_PROP" ] || abort "活动元模块缺少 module.prop，安装已中断"
[ ! -e "$METAMODULE_DIR/disable" ] || abort "当前元模块已禁用，安装已中断"
[ ! -e "$METAMODULE_DIR/remove" ] || abort "当前元模块正等待卸载，安装已中断"
grep -Eq '^metamodule=(true|1)$' "$METAMODULE_PROP" || abort "当前活动模块不是有效元模块，安装已中断"
metamodule_name=$(sed -n 's/^name=//p' "$METAMODULE_PROP" | head -n 1)
metamodule_version=$(sed -n 's/^version=//p' "$METAMODULE_PROP" | head -n 1)
ui_print "- 活动元模块：${metamodule_name:-未知} ${metamodule_version:-}"

DATA=/data/adb/limbus-localization
SYSTEM_CA="$DATA/system-ca"
MODULE_CA="$MODPATH/system/etc/security/cacerts"

if [ ! -s "$DATA/ssl/ca.crt" ] || [ ! -s "$DATA/ssl/ca.key" ]; then
    ui_print ""
    ui_print "本模块将生成设备专属 CA，并通过 systemless 挂载加入系统信任库。"
    ui_print "不会写入 system 分区，也不会删除任何系统证书。"
    ui_print ""
    ui_print "  音量 +：确认并继续"
    ui_print "  音量 -：取消安装"

    while true; do
        key=$(getevent -qlc 1 2>/dev/null)
        case "$key" in
            *KEY_VOLUMEUP*DOWN*) break ;;
            *KEY_VOLUMEDOWN*DOWN*) abort "用户取消安装" ;;
        esac
    done
else
    ui_print "- 复用本机已有 CA，不重复请求确认"
fi

set_perm "$MODPATH/bin/limbus-proxy" 0 0 0755
set_perm "$MODPATH/post-fs-data.sh" 0 0 0755
set_perm "$MODPATH/post-mount.sh" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/action.sh" 0 0 0755
set_perm "$MODPATH/status.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755

mkdir -p "$DATA" "$SYSTEM_CA" "$MODULE_CA" || abort "无法创建模块目录"
chmod 0700 "$DATA"
chmod 0755 "$MODPATH/system" "$MODPATH/system/etc" \
    "$MODPATH/system/etc/security" "$MODULE_CA"

ui_print "- 生成设备专属证书"
"$MODPATH/bin/limbus-proxy" init \
    --data "$DATA" \
    --cert-dir "$SYSTEM_CA" || abort "证书生成失败"
rm -f "$MODULE_CA"/*.0
cp -af "$SYSTEM_CA"/*.0 "$MODULE_CA/" || abort "无法准备系统证书挂载"
for cert in "$MODULE_CA"/*.0; do
    set_perm "$cert" 0 0 0644 u:object_r:system_security_cacerts_file:s0
done

ui_print "- 下载并校验最新汉化资源"
if ! "$MODPATH/bin/limbus-proxy" update --data "$DATA"; then
    ui_print "! 首次下载失败；重启后可从模块管理器再次执行更新"
fi

ui_print "- 安装完成，请重启设备"
