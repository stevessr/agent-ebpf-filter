# Probe manager integration sketch

A later Go integration can run bpf-ts at build time, embed the resulting object, read the versioned manifest, and dispatch attach operations through the existing probe manager. This keeps runtime capture lifecycle, PID identity/backoff and link cleanup in the existing Go layer rather than reimplementing them in TypeScript.
