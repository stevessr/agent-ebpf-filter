# AgentSight utility tests

These tests run outside the Vue application TypeScript project so Bun-specific test globals do not leak into production type checking.

The TLS/AgentSight smoke workflow runs the stdio framing suite before `vue-tsc`/Vite build. Stateful stdio tests cover byte-counted `Content-Length` framing, LSP/MCP protocol classification, cross-event reassembly, stream isolation, truncation reset, and bounded pending buffers.
