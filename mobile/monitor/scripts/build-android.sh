#!/usr/bin/env bash
# Build an arm64 debug APK using the Android host shipped by MyGo v0.10+.
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
NDK_DIR="$SDK/ndk/$NDK_VERSION"
export CC="${MYGO_ANDROID_CC:-$NDK_DIR/toolchains/llvm/prebuilt/$HOST_TAG/bin/aarch64-linux-android29-clang}"
if [ ! -x "$CC" ]; then
  echo "Missing Android NDK clang: $CC" >&2
  exit 2
fi

go mod download github.com/egoist/mygo
MYGO_ROOT="$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)"
ANDROID_HOST_SRC="$MYGO_ROOT/internal/mobile/android"
if [ ! -f "$ANDROID_HOST_SRC/app/build.gradle.kts" ]; then
  echo "The pinned MyGo module does not contain internal/mobile/android: $MYGO_ROOT" >&2
  exit 2
fi

# 16 KiB page-size devices require 16 KiB load segment alignment.
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-z,max-page-size=16384"

STAGE="$APP/.mygo/android-build"
WORK="$STAGE/work"
OUT="$APP/build/android-arm64"
mkdir -p "$STAGE" "$WORK" "$OUT"

python3 - "$ANDROID_HOST_SRC" "$STAGE" "$WORK" "$ROOT" <<'PY'
import json
import os
import re
import shutil
import stat
import sys
from pathlib import Path

host_src, stage, work, repo = map(Path, sys.argv[1:])
config = json.loads(Path("mygo.json").read_text(encoding="utf-8"))
identifier = config["identifier"]
if not re.fullmatch(r"[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+", identifier):
    raise SystemExit("Invalid Android application ID")
version = config["version"]
if not re.fullmatch(r"[0-9][A-Za-z0-9.+-]*", version):
    raise SystemExit("Invalid Android version")
schemes = config.get("urlSchemes") or []
for scheme in schemes:
    if not re.fullmatch(r"[a-z][a-z0-9+.-]*", scheme):
        raise SystemExit(f"Invalid Android URL scheme: {scheme}")

host = stage / "host"
# Each invocation starts from the pinned host rather than reusing generated
# Gradle/JNI resources from a previous build in the same checkout.
if host.exists():
    shutil.rmtree(host)
if work.exists():
    shutil.rmtree(work)
shutil.copytree(
    host_src,
    host,
    ignore=shutil.ignore_patterns("build", ".gradle", ".kotlin", "jniLibs", "local.properties", "embed.go"),
)
for directory, subdirs, files in os.walk(host):
    for name in [directory] + [os.path.join(directory, f) for f in files]:
        if not os.path.islink(name):
            os.chmod(name, os.stat(name).st_mode | stat.S_IWUSR)

res = work / "res"
drawable = res / "drawable"
xml_dir = res / "xml"
assets = work / "assets" / "resources"
drawable.mkdir(parents=True, exist_ok=True)
xml_dir.mkdir(parents=True, exist_ok=True)
assets.mkdir(parents=True, exist_ok=True)
shutil.copyfile(repo / "desktop/renew/resources/icon.png", drawable / "renew_monitor_icon.png")

# Disallow remote cleartext HTTP at the Android platform layer while permitting
# localhost loopback for USB `adb reverse` debugging.
(xml_dir / "network_security_config.xml").write_text(
    """<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
    <base-config cleartextTrafficPermitted="false" />
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">localhost</domain>
        <domain includeSubdomains="false">127.0.0.1</domain>
        <domain includeSubdomains="false">::1</domain>
    </domain-config>
</network-security-config>
""",
    encoding="utf-8",
)

scheme_filters = []
for scheme in schemes:
    scheme_filters.append(
        f"""            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="{scheme}" />
            </intent-filter>"""
    )
scheme_block = ("\n" + "\n".join(scheme_filters)) if scheme_filters else ""

# A remote monitor needs no camera, microphone, location, media, foreground
# service or push notification permissions. Retain only network access.
(work / "AndroidManifest.xml").write_text(
    f"""<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
    <application
        android:label="${{mygoAppName}}"
        android:icon="${{mygoIcon}}"
        android:networkSecurityConfig="@xml/network_security_config"
        android:supportsRtl="true"
        android:enableOnBackInvokedCallback="true"
        android:theme="@style/Theme.MyGo">
        <activity
            android:name="dev.mygo.MyGoActivity"
            android:exported="true"
            android:launchMode="singleTask"
            android:configChanges="orientation|screenSize|screenLayout|smallestScreenSize|keyboard|keyboardHidden|navigation|uiMode|fontScale|density|locale|layoutDirection"
            android:windowSoftInputMode="adjustResize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>{scheme_block}
        </activity>
    </application>
</manifest>
""",
    encoding="utf-8",
)
PY

printf 'sdk.dir=%s\n' "$SDK" > "$STAGE/host/local.properties"
JNI_DIR="$WORK/jniLibs/arm64-v8a"
mkdir -p "$JNI_DIR"
echo "Compiling Go Android arm64 native library..."
SO="$JNI_DIR/libmygoapp.so"
GOOS=android GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w" \
  -o "$SO" .
rm -f "${SO%.so}.h"
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

APP_NAME="$(python3 -c 'import json; print(json.load(open("mygo.json"))["name"])')"
APP_ID="$(python3 -c 'import json; print(json.load(open("mygo.json"))["identifier"])')"
APP_VERSION="$(python3 -c 'import json; print(json.load(open("mygo.json"))["version"])')"

chmod +x "$STAGE/host/gradlew"
ORG_GRADLE_PROJECT_mygoApplicationId="$APP_ID" \
ORG_GRADLE_PROJECT_mygoAppName="$APP_NAME" \
ORG_GRADLE_PROJECT_mygoVersionName="$APP_VERSION" \
ORG_GRADLE_PROJECT_mygoVersionCode="100" \
ORG_GRADLE_PROJECT_mygoMinSdk="29" \
ORG_GRADLE_PROJECT_mygoIcon="@drawable/renew_monitor_icon" \
ORG_GRADLE_PROJECT_mygoJniLibs="$WORK/jniLibs" \
ORG_GRADLE_PROJECT_mygoAssets="$WORK/assets" \
ORG_GRADLE_PROJECT_mygoRes="$WORK/res" \
ORG_GRADLE_PROJECT_mygoManifest="$WORK/AndroidManifest.xml" \
ORG_GRADLE_PROJECT_mygoAbis="arm64-v8a" \
ORG_GRADLE_PROJECT_mygoNdk="$NDK_DIR" \
  "$STAGE/host/gradlew" -p "$STAGE/host" :app:assembleDebug --no-daemon --stacktrace --console=plain

SOURCE_APK="$STAGE/host/app/build/outputs/apk/debug/app-debug.apk"
test -s "$SOURCE_APK" || { echo "Gradle did not produce a debug APK" >&2; exit 1; }
cp "$SOURCE_APK" "$OUT/RenewMonitor-debug.apk"
unzip -tq "$OUT/RenewMonitor-debug.apk" >/dev/null
echo "APK: $OUT/RenewMonitor-debug.apk"
if [ "$INSTALL" -eq 1 ]; then
  "$SDK/platform-tools/adb" install -r "$OUT/RenewMonitor-debug.apk"
  "$SDK/platform-tools/adb" shell am start -W -n "$APP_ID/dev.mygo.MyGoActivity"
fi
