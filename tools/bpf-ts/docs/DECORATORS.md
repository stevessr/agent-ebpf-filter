# Probe decorators

- `@kprobe("symbol")`
- `@uprobe("symbol")`
- `@tracepoint("category", "event")`

Each probe function accepts exactly one context parameter and returns an integer-compatible value.
