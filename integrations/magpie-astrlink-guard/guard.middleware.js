import { collectTextSlots, maskDeterministic } from "./redaction_core.js";

// Synchronous, deterministic fall-back for the async AstrLink model hook.
// This is intentionally cheap: no network, model loading or raw-content logs.
export function onRequest(body, ctx) {
  const options = ctx.options ?? {};
  const action = ["audit", "redact"].includes(options.action) ? options.action : "redact";
  if (action === "audit") return;
  const { slots, overLimit } = collectTextSlots(body, options);
  if (overLimit) return ctx.reject(413, "Privacy scan input exceeds configured limits");
  maskDeterministic(body, options);
  return body;
}
