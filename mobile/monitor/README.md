# Renew Monitor · MyGo Mobile

A **read-only** Android companion for an existing Agent eBPF collector. The
Android client connects to the backend; eBPF still runs on the remote Linux
machine and is **not** installed or started on the phone.

This is a small, independently testable mobile client, not a resized desktop
window. The native touch UI is written in Go using
[MyGo Mobile](https://github.com/daodao97/mygo-mobile), which adds Android and
iOS backends to [egoist/mygo](https://github.com/egoist/mygo).
The project pins the fork to an exact pseudo-version in `go.mod` instead of
replacing the desktop's MyGo dependency.

## First version

- HTTPS origin and token-based connection; the token remains in app memory
  and is cleared when disconnecting, not written to disk.
- Collector health: live capture flag, dropped ring-buffer events, queue and
  persistence backlog.
- Recent 60 events: PID, command, agent tag, target, risk score, decision.
- Local search over the current event snapshot; individual event detail GET
  requests only when tapped.
- Foreground polling every six seconds; no background service or push
  notifications, and no policy, model, file, or sandbox mutation operations.
- Refresh and disconnect, with bounded network timeouts and error display.

Protocol: `GET /system/collector-health`, `GET /events/summaries?compact=1&limit=60`,
and optionally `GET /events/detail/:eventID`. Authentication matches the Renew
desktop client: `X-API-KEY` and `Authorization: Bearer ...`.

## Connect from Android

1. Start an Agent eBPF backend on a Linux host with API authentication.
   The desktop's **embedded** backend normally listens on a private Unix
   socket, so it is **not** reachable from Android. Run a separately
   configured service that exposes a reachable TCP listener.
2. Put it behind a **trusted HTTPS reverse proxy** or a VPN endpoint with
   valid TLS. Prefer a separately scoped read-only credential if the server
   supports it; currently the shared backend token can authorize more than
   this client uses.
3. Install the APK, type the full origin (for example,
   `https://monitor.example.com`) and the server's API Token, then connect.

Only HTTPS is accepted remotely; HTTP is permitted for local loopback tests,
not for LAN or WAN connections. The Android network security policy from
MyGo Mobile likewise disallows cleartext traffic to remote hosts.
The system certificate store is used; invalid/self-signed certificates are
not silently trusted. Avoid exposing the collector API directly to the public
Internet or opening extra firewall ports just to use the companion.

## Development

From `mobile/monitor`:

```bash
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

Go 1.27.3 is required by the pinned MyGo fork. The mobile module is isolated
from `backend` and `desktop/renew`, so building the app never bundles the
privileged eBPF backend.

## Android debug APK

Requirements: Linux/macOS, JDK 17, Android SDK API 36, NDK
`28.2.13676358`, Go 1.27.3, Gradle (the MyGo host bundles its wrapper),
and the Android HarfBuzz/FreeType static archives. Set
`ANDROID_HOME` and `MYGO_HB_ANDROID` (the latter points to the `android`
directory containing `include/` and `lib/`).

```bash
bash mobile/monitor/scripts/build-android.sh
# optionally install to a device:
bash mobile/monitor/scripts/build-android.sh --install
```

Output: `mobile/monitor/build/android-arm64/RenewMonitor-debug.apk`.
The build stages a **fresh** Android host from the pinned MyGo module on each
invocation and validates that Gradle actually generated a nonempty ZIP/APK.
Native `.so` load segments use Android's 16 KiB alignment via NDK r28.
The Android manifest strips unrelated runtime permissions.

Validate a local APK using the same checks as CI:

```bash
bash mobile/monitor/scripts/verify-android-apk.sh
# or pass an APK path as the first argument
```

This requires Android SDK Build Tools 36.0.0 (or set
`ANDROID_BUILD_TOOLS_VERSION`) and checks the installable package name,
minimum SDK, INTERNET permission, JNI arm64 library, debug signature and
16 KiB ZIP alignment. It generates a `SHA256SUMS` alongside the verified APK.

## GitHub Actions and test releases

The [Renew mobile Android](../../.github/workflows/renew-mobile.yml)
workflow runs mobile Go tests, race tests, vet, the native Android build,
the APK acceptance checks, and uploads both APK and checksum to the
`renew-monitor-android-arm64-debug` Actions artifact on success.

On a successful **master** branch push affecting the mobile module, or a
manual run of that workflow against `master`, a separate publish job creates
an immutable GitHub **prerelease**, tagged `mobile-test-<workflow-run-id>`.
Find it on the [Releases page](https://github.com/stevessr/agent-ebpf-filter/releases).
Pull-request builds never publish. The prerelease is kept separate from
the stable Linux release and explicitly excluded from the latest stable
release selection.

This is a **debug-signed, experimental test APK** — not a production/store
release. Each GitHub-hosted runner may use a different debug signing key;
if Android refuses to upgrade an existing test APK, uninstall the prior
test build before installing the new one. Uninstalling clears local app
state. For repeatable in-place upgrades or production distribution, configure
a protected persistent release-signing key and a separate signing workflow.

The CI checks establish build/package integrity, not real-device behavior.
Before treating the APK as stable, test launch, touch, text input, rotation,
pause/resume, valid/invalid HTTPS authentication and network loss on an
actual Android device.

### Current limitations

- No iOS IPA, background alerts, push notifications, event pagination, or
  model/agent/policy configuration editing yet.
- A backend launched only in the desktop's local Unix-socket mode cannot be
  reached remotely. Connection failures in that setup are expected.
- This client performs GET-only operations but cannot enforce server-side
  read-only authorization. A server-scoped read-only token/API role is a
  separate backend enhancement.
- The PINNED community MyGo fork is experimental on Android and needs a
  real-device check (startup, touch, keyboard, rotation, foreground/background,
  network error handling) before release distribution.
