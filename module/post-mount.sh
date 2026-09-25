#!/system/bin/sh

DATA=/data/adb/limbus-localization
RUNTIME=/dev/limbus-localization
SOURCE=/system/etc/hosts
TARGET="$RUNTIME/hosts"
LOG="$DATA/logs/post-mount.log"

mkdir -p "$DATA/logs" "$RUNTIME"
exec >>"$LOG" 2>&1

[ -f "$SOURCE" ] || exit 1
cp -af "$SOURCE" "$TARGET.new" || exit 1
if ! grep -q '^127\.0\.0\.1[[:space:]][[:space:]]*downloadcommon\.limbuscompanycdn\.org$' "$TARGET.new"; then
    printf '\n127.0.0.1 downloadcommon.limbuscompanycdn.org\n' >> "$TARGET.new" || exit 1
fi

chown 0:0 "$TARGET.new" || exit 1
chmod 0644 "$TARGET.new" || exit 1
chcon --reference="$SOURCE" "$TARGET.new" || exit 1
mv -f "$TARGET.new" "$TARGET" || exit 1
mount --bind "$TARGET" "$SOURCE" || exit 1
echo "hosts mounted: $(date)"
