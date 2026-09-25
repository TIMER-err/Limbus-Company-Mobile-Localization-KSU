#!/system/bin/sh

MODDIR=${0%/*}
DATA=/data/adb/limbus-localization
LOG="$DATA/logs/post-fs-data.log"
mkdir -p "$DATA/logs"
exec >>"$LOG" 2>&1

sdk=$(getprop ro.build.version.sdk)
[ "$sdk" -ge 34 ] || exit 0

source_dir=/apex/com.android.conscrypt/cacerts
runtime_dir="$MODDIR/.conscrypt-cacerts"
custom_dir="$DATA/system-ca"

[ -d "$source_dir" ] || exit 0
rm -rf "$runtime_dir"
mkdir -p "$runtime_dir"
mount -t tmpfs -o mode=0755 tmpfs "$runtime_dir" || exit 1

fail_mount() {
    echo "Conscrypt CA mount failed: $1"
    umount "$runtime_dir" 2>/dev/null
    exit 1
}

cp -af "$source_dir/." "$runtime_dir/" || fail_mount "copy system store"
cp -af "$custom_dir/"*.0 "$runtime_dir/" || fail_mount "copy module CA"

chown 0:2000 "$runtime_dir" || fail_mount "set directory owner"
chmod 0755 "$runtime_dir" || fail_mount "set directory mode"
chown 1000:1000 "$runtime_dir/"*.0 || fail_mount "set certificate owner"
chmod 0644 "$runtime_dir/"*.0 || fail_mount "set certificate mode"
chcon u:object_r:system_security_cacerts_file:s0 "$runtime_dir" "$runtime_dir/"*.0 || fail_mount "set SELinux label"

mount --bind "$runtime_dir" "$source_dir" || fail_mount "bind store"
echo "Conscrypt CA mounted: $(date)"
