#!/usr/bin/env bash
# CI-only exploratory measurements. All long-lived kernel changes stay in this ephemeral runner.
set -euo pipefail
mkdir -p reports/paper-ci/{off,on,context,enforcement}
out="$PWD/reports/paper-ci"
{
  date -u +"%Y-%m-%dT%H:%M:%SZ"
  git rev-parse HEAD
  uname -a
  lscpu | grep -E 'Model name|CPU\(s\)|Core\(s\)|Socket\(s\)' || true
  go version
  id
  echo "Active LSM list:"
  cat /sys/kernel/security/lsm || true
  echo "BTF available:"; test -r /sys/kernel/btf/vmlinux && echo yes || echo no
  echo "Cgroup type:"; stat -f -c %T /sys/fs/cgroup || true
  echo "Cgroup writable:"; test -w /sys/fs/cgroup && echo yes || echo no
} > "$out/host.txt" 2>&1

echo "[paper-ci] build standalone read(2) sampler"
go build -o "$RUNNER_TEMP/paper-read-sampler" scripts/paper-syscall-latency.go
collect_samples() {
  local mode="$1"
  for concurrency in 1 4 8 16 32; do
    for cycle in $(seq 1 10); do
      "$RUNNER_TEMP/paper-read-sampler" --concurrency "$concurrency" --samples 20000 \
        --warmup 200 --output "$out/$mode/read_c$concurrency-r$cycle.csv" >/dev/null
    done
    echo "[paper-ci] $mode C=$concurrency: 10 x 20000 measured read syscalls"
  done
}

echo "[paper-ci] OFF control measurements (before our backend starts)"
collect_samples off

echo "[paper-ci] install eBPF build requirements"
sudo apt-get update -qq
sudo apt-get install -qq -y clang llvm libbpf-dev libelf-dev protobuf-compiler jq
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
mkdir -p backend/pb
protoc --go_out=backend/pb --go_opt=paths=source_relative -I proto \
  proto/tracker_common.proto proto/tracker_events.proto \
  proto/tracker_registration.proto proto/tracker_system.proto \
  proto/tracker_config.proto proto/tracker_shell.proto
(cd backend/ebpf && go generate)
go build -o "$RUNNER_TEMP/paper-backend" ./backend
(cd scripts && go build -o benchmark-syscalls-extended ./benchmark-syscalls-extended.go)

echo "[paper-ci] start privileged backend and verify attached kernel hooks"
sudo -E env DISABLE_AUTH=true GIN_MODE=release "$RUNNER_TEMP/paper-backend" \
  > "$out/backend.log" 2>&1 &
pid=$!
trap 'sudo kill "$pid" 2>/dev/null || true' EXIT
ready=0
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/system/bootstrap-health > "$out/health.json" 2>/dev/null && \
      jq -e '.attachedCount >= 1' "$out/health.json" >/dev/null; then
    ready=1; break
  fi
  sleep 2
done
if [[ "$ready" != 1 ]]; then
  echo "SKIP: cannot verify attached project eBPF hooks" | tee "$out/skip-reason.txt"
  tail -50 "$out/backend.log" || true
  exit 0
fi
jq '{status,compiledCount,attachedCount,skippedCount}' "$out/health.json"
collect_samples on

echo "[paper-ci] real Agent root registration + same-name tools"
python3 scripts/attribution-concurrent-workload.py \
  --agents 2 3 4 --cycles 10 --backend-url http://127.0.0.1:8080 \
  --repo "$PWD" --output "$out/context/ground-truth.csv" \
  > "$out/context/workload.log" 2>&1 || {
    echo "Agent ground-truth workload failed (non-fatal; evidence in logfile)"
    tail -25 "$out/context/workload.log"
  }

echo "[paper-ci] existing 10-cycle concurrency benchmark (same-attached-state drift only)"
BENCH_CONCURRENCY="1 4 8 16 32" BENCH_CYCLES=10 \
  EBPF_BENCH_OUTDIR="$out/legacy-concurrent" \
  bash scripts/benchmark-concurrent.sh > "$out/legacy.log" 2>&1 || {
    echo "Existing concurrent benchmark failed; inspect artifact legacy.log"
    tail -50 "$out/legacy.log"
  }

curl -fsS http://127.0.0.1:8080/sandbox/cgroup/status \
  > "$out/enforcement/cgroup-status.json" || true
curl -fsS http://127.0.0.1:8080/sandbox/lsm/status \
  > "$out/enforcement/lsm-status.json" || true
if jq -e '.available == true and .attached == true' \
  "$out/enforcement/cgroup-status.json" >/dev/null 2>&1 && \
  jq -e '.available == true and .attached == true' \
  "$out/enforcement/lsm-status.json" >/dev/null 2>&1; then
  echo "[paper-ci] cgroup and LSM both attached; run actual errno probes"
  curl -fsS -X PUT http://127.0.0.1:8080/config/runtime \
    -H 'Content-Type: application/json' \
    -d '{"policyManagementEnabled":true}' \
    > "$out/enforcement/enable-policy.json" || true
  python3 scripts/paper-enforcement-trials.py --trials 20 \
    --backend-url http://127.0.0.1:8080 \
    --output "$out/enforcement/enforcement.csv" \
    > "$out/enforcement/trials.log" 2>&1 || {
      echo "Kernel deny probes incomplete; see trials.log"
      tail -45 "$out/enforcement/trials.log"
    }
else
  echo "SKIPPED: BPF LSM or cgroup attachment unavailable" \
    | tee "$out/enforcement/skip-reason.txt"
fi
echo "[paper-ci] finished, raw evidence in $out"
