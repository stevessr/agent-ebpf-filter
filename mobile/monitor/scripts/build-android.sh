#!/usr/bin/env bash
# Build an arm64 debug APK using the Android host shipped by MyGo Mobile.
# Usage: bash mobile/monitor/scripts/build-android.sh [--install] [--so-only]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd -P)"
APP="$ROOT/mobile/monitor"
export GOWORK=off
cd "$APP"

INSTALL=0
SO_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --install) INSTALL=1 ;;
    --so-only) SO_ONLY=1 ;;
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

case "$(uname -s)" in
  Linux) HOST_TAG=linux-x86_64; SDK_DEFAULT="$HOME/Android/Sdk" ;;
  Darwin) HOST_TAG=darwin-x86_64; SDK_DEFAULT="$HOME/Library/Android/sdk" ;;
  *) echo "Building Android APKs requires a Linux or macOS host" >&2; exit 2 ;;
esac

SDK="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$SDK_DEFAULT}}"
NDK_VERSION="${ANDROID_NDK_VERSION:-28.2.13676358}"
export CC="${MYGO_ANDROID_CC:-$SDK/ndk/$NDK_VERSION/toolchains/llvm/prebuilt/$HOST_TAG/bin/aarch64-linux-android29-clang}"
if [ ! -x "$CC" ]; then
  echo "Missing Android NDK clang: $CC" >&2
  exit 2
fi

# The replaced upstream import resolves to the pinned MyGo Mobile fork.
go mod download github.com/egoist/mygo
MYGO_ROOT="$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)"
if [ ! -f "$MYGO_ROOT/android/app/build.gradle" ]; then
  echo "The pinned MyGo module does not contain its Android host: $MYGO_ROOT" >&2
  exit 2
fi
HB_ROOT="${MYGO_HB_ANDROID:-$MYGO_ROOT/third_party/hb-build/src/lib/android}"
if [ ! -f "$HB_ROOT/lib/libharfbuzz.a" ] || [ ! -f "$HB_ROOT/lib/libfreetype.a" ]; then
  cat >&2 <<EOF
MyGo Android requires HarfBuzz and FreeType static archives.
Expected: $HB_ROOT/lib/libharfbuzz.a and $HB_ROOT/lib/libfreetype.a
Set MYGO_HB_ANDROID to a directory containing include/ and lib/.
The MyGo Mobile upstream documents its prebuilt Android archives.
EOF
  exit 2
fi
export CGO_CFLAGS="${CGO_CFLAGS:-} -I$HB_ROOT/include/harfbuzz -I$HB_ROOT/include/freetype2"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L$HB_ROOT/lib -lharfbuzz -lfreetype -lm -llog"

STAGE="$APP/.mygo/android-build"
OUT="$APP/build/android-arm64"
mkdir -p "$STAGE" "$OUT"

python3 - "$MYGO_ROOT" "$STAGE" "$ROOT" <<'PY'
import json
import os
import re
import shutil
import stat
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

framework, stage, repo = map(Path, sys.argv[1:])
config = json.loads(Path("mygo.json").read_text(encoding="utf-8"))
identifier = config["identifier"]
if not re.fullmatch(r"[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+", identifier):
    raise SystemExit("Invalid Android application ID")
version = config["version"]
if not re.fullmatch(r"[0-9][A-Za-z0-9.+-]*", version):
    raise SystemExit("Invalid Android version")

host = stage / "host"
shutil.copytree(
    framework / "android", host,
    dirs_exist_ok=True,
    ignore=shutil.ignore_patterns("build", ".gradle", ".kotlin", "jniLibs", "local.properties"),
)
# Go's module cache is read-only; Gradle must write into the staged copy.
for directory, subdirs, files in os.walk(host):
    for name in [directory] + [os.path.join(directory, f) for f in files]:
        if not os.path.islink(name):
            os.chmod(name, os.stat(name).st_mode | stat.S_IWUSR)
gradle = host / "app/build.gradle"
text = gradle.read_text()
for old, new in [
    ("applicationId 'org.mygo.host'", "applicationId '" + identifier + "'"),
    ("versionName '1.0'", "versionName '" + version + "'"),
    ("minSdk 24", "minSdk 29"),
]:
    if old not in text:
        raise SystemExit("MyGo Android host changed: missing " + old)
    text = text.replace(old, new)
gradle.write_text(text)

ET.register_namespace("android", "http://schemas.android.com/apk/res/android")
ns = "{http://schemas.android.com/apk/res/android}"
manifest = host / "app/src/main/AndroidManifest.xml"
tree = ET.parse(manifest)
root = tree.getroot()
# A remote monitor needs no camera, microphone, location, media, foreground
# service or push permissions. Retain only the essential network access.
for element in list(root):
    if element.tag == "uses-permission" and element.get(ns + "name") not in {
        "android.permission.INTERNET", "android.permission.ACCESS_NETWORK_STATE",
    }:
        root.remove(element)
app = root.find("application")
if app is None:
    raise SystemExit("MyGo Android manifest is missing application")
app.set(ns + "label", config["name"])
app.set(ns + "icon", "@drawable/renew_monitor_icon")
tree.write(manifest, encoding="utf-8", xml_declaration=True)
icons = host / "app/src/main/res/drawable"
icons.mkdir(parents=True, exist_ok=True)
shutil.copyfile(repo / "desktop/renew/resources/icon.png", icons / "renew_monitor_icon.png")
PY

printf 'sdk.dir=%s\n' "$SDK" > "$STAGE/host/local.properties"
mkdir -p "$STAGE/host/app/src/main/jniLibs/arm64-v8a"
echo "Compiling Go Android arm64 native library..."
SO="$STAGE/host/app/src/main/jniLibs/arm64-v8a/libmygo.so"
GOOS=android GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared \
  -o "$SO" .
# 16 KiB page-size devices require each loadable segment to be at least
# 16 KiB aligned. Check the actual produced library, not just the NDK version.
OBJDUMP="$(dirname "$CC")/llvm-objdump"
test -x "$OBJDUMP" || { echo "Missing NDK llvm-objdump: $OBJDUMP" >&2; exit 2; }
LOADS="$("$OBJDUMP" -p "$SO" | grep -w LOAD || true)"
if [ -z "$LOADS" ] || printf '%s\n' "$LOADS" | grep -Fqv 'align 2**14'; then
  echo "Android library does not meet 16 KiB ELF load segment alignment" >&2
  printf '%s\n' "$LOADS" >&2
  exit 1
fi
echo "Verified 16 KiB aligned ELF load segments."
if [ "$SO_ONLY" -eq 1 ]; then
  echo "Native .so ready (APK assembly skipped)"
  exit 0
fi
chmod +x "$STAGE/host/gradlew"
"$STAGE/host/gradlew" -p "$STAGE/host" :app:assembleDebug --console=plain
cp "$STAGE/host/app/build/outputs/apk/debug/app-debug.apk" "$OUT/RenewMonitor-debug.apk"
echo "APK: $OUT/RenewMonitor-debug.apk"
if [ "$INSTALL" -eq 1 ]; then
  "$SDK/platform-tools/adb" install -r "$OUT/RenewMonitor-debug.apk"
  "$SDK/platform-tools/adb" shell am start -W -n "dev.agentebpf.monitor/org.mygo.MygoActivity"
fi
