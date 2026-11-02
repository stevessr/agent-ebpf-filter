/**
 * Pure keyboard decision for the Web workbench.
 *
 * Avoid stealing host/editor shortcuts and keep Escape available for closing
 * the narrow navigation overlay even while its shell search input has focus.
 */
export type WorkbenchShortcut = "focus-search" | "close-overlay";

export interface WorkbenchShortcutContext {
  key: string;
  ctrlKey?: boolean;
  metaKey?: boolean;
  defaultPrevented?: boolean;
  inEditorOrTerminal?: boolean;
  inEditableField?: boolean;
  narrow: boolean;
  sidebarOpen: boolean;
}

export function resolveWorkbenchShortcut(ctx: WorkbenchShortcutContext): WorkbenchShortcut | null {
  if (ctx.defaultPrevented || ctx.inEditorOrTerminal) return null;
  if (ctx.key === "Escape" && ctx.narrow && ctx.sidebarOpen) return "close-overlay";
  if (ctx.inEditableField) return null;
  if ((ctx.ctrlKey || ctx.metaKey) && ctx.key.toLowerCase() === "k") return "focus-search";
  return null;
}
