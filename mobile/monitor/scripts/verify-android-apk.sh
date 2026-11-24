#!/usr/bin/env bash
# Validate an actual installable Android arm64 debug APK before distribution.
# Usage: bash mobile/monitor/scripts/verify-android-apk.sh [path-to-apk]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
APK="${1:-$ROOT/mobile/monitor/build/android-arm64/RenewMonitor-debug.apk}"
SDK="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}"
BUILD_TOOLS="$SDK/build-tools/${ANDROID_BUILD_TOOLS_VERSION:-36.0.0}"

if [[ ! -s "$APK" ]]; then
  echo "APK is missing or empty: $APK" >&2
  exit 1
fi
for tool in aapt apksigner zipalign; do
  if [[ ! -x "$BUILD_TOOLS/$tool" ]]; then
    echo "Missing Android SDK build tool: $BUILD_TOOLS/$tool" >&2
    exit 2
  fi
done

# A successful Gradle build alone does not prove the uploaded file has the
# expected package, native library, signature, or Android 16 KiB zip alignment.
unzip -tq "$APK" >/dev/null
if ! unzip -Z1 "$APK" | grep -Ex 'lib/arm64-v8a/libmygo(app)?\.so' >/dev/null; then
  echo "APK does not bundle the MyGo arm64 JNI library" >&2
  exit 1
fi

BADGING="$(mktemp)"
trap 'rm -f "$BADGING"' EXIT
"$BUILD_TOOLS/aapt" dump badging "$APK" > "$BADGING"
grep -F "package: name='dev.agentebpf.monitor'" "$BADGING" || {
  echo "Unexpected APK package/application ID" >&2
  exit 1
}
grep -F "sdkVersion:'29'" "$BADGING" || {
  echo "Unexpected minimum Android SDK version" >&2
  exit 1
}
grep -F "uses-permission: name='android.permission.INTERNET'" "$BADGING" || {
  echo "APK is missing the required network permission" >&2
  exit 1
}

"$BUILD_TOOLS/apksigner" verify --min-sdk-version 29 --verbose --print-certs "$APK"
"$BUILD_TOOLS/zipalign" -c -P 16 -v 4 "$APK"

# Always checksum exactly the APK we just verified, using a portable relative
# filename so that the checksum also works after download from Actions/Release.
(
  cd "$(dirname "$APK")"
  sha256sum "$(basename "$APK")" > SHA256SUMS
  sha256sum -c SHA256SUMS
)
echo "Verified installable Android arm64 debug APK: $APK"
