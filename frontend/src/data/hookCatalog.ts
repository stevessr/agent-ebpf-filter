import type { HookCliDoc } from "./hooks/types";
import { claudeHook } from "./hooks/claude";
import { geminiHook } from "./hooks/gemini";
import { codexHook } from "./hooks/codex";
import { kiroHook } from "./hooks/kiro";
import { augmentHook } from "./hooks/augment";
import { antigravityHook } from "./hooks/antigravity";
import { copilotHook } from "./hooks/copilot";
import { dshHook } from "./hooks/dsh";
import { piHook } from "./hooks/pi";
import { ompHook } from "./hooks/omp";

export type { HookCliDoc, HookEventDoc, HookFieldDoc, HookSourceDoc } from "./hooks/types";

export const hookCatalog: Record<string, HookCliDoc> = {
	claude: claudeHook,
	gemini: geminiHook,
	codex: codexHook,
	kiro: kiroHook,
	augment: augmentHook,
	antigravity: antigravityHook,
	copilot: copilotHook,
	dsh: dshHook,
	pi: piHook,
	omp: ompHook,
};

export const getHookCliDoc = (hookId?: string | null) => {
  if (!hookId) return null;
  return hookCatalog[hookId] ?? null;
};

export const getHookEventDoc = (
  hookId: string | undefined,
  eventName: string | undefined,
) => {
  const cliDoc = getHookCliDoc(hookId);
  if (!cliDoc || !eventName) return null;
  const normalized = eventName.trim().toLowerCase();
  return (
    cliDoc.events.find((event) => {
      if (event.name.toLowerCase() === normalized) return true;
      return (event.aliases || []).some(
        (alias) => alias.toLowerCase() === normalized,
      );
    }) ?? null
  );
};
