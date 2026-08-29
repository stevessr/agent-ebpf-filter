# Attach manifest

The compiler can emit a JSON manifest containing probe names, attach kinds and targets plus map metadata. This keeps loader decisions out of generated C comments and gives later Go integration a stable machine-readable handoff.
