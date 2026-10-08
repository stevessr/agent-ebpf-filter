/** Serializable docking tree. No API data or credentials are persisted here. */
export interface DockTab {
  id: string;
  path: string;
  title: string;
}
export interface DockGroup {
  kind: "group";
  id: string;
  tabs: DockTab[];
  active: string;
}
export interface DockSplit {
  kind: "split";
  id: string;
  axis: "horizontal" | "vertical";
  ratio: number;
  first: DockNode;
  second: DockNode;
}
export type DockNode = DockGroup | DockSplit;
export type DockEdge = "center" | "left" | "right" | "top" | "bottom";
let nextId = 0;
// randomUUID is unavailable on non-secure LAN HTTP origins. These are UI IDs,
// not authentication tokens, so a local counter/random fallback is sufficient.
export const uid = () =>
  globalThis.crypto?.randomUUID?.() ||
  `dock-${Date.now().toString(36)}-${++nextId}-${Math.random().toString(36).slice(2)}`;
export const group = (tabs: DockTab[] = []): DockGroup => ({
  kind: "group",
  id: uid(),
  tabs,
  active: tabs[0]?.id || "",
});
export function groups(node: DockNode): DockGroup[] {
  return node.kind === "group"
    ? [node]
    : [...groups(node.first), ...groups(node.second)];
}
export function replaceNode(
  node: DockNode,
  id: string,
  next: DockNode,
): DockNode {
  if (node.id === id) return next;
  if (node.kind === "group") return node;
  return {
    ...node,
    first: replaceNode(node.first, id, next),
    second: replaceNode(node.second, id, next),
  };
}
export function prune(node: DockNode): DockNode | null {
  if (node.kind === "group") return node.tabs.length ? node : null;
  const first = prune(node.first),
    second = prune(node.second);
  return first && second ? { ...node, first, second } : first || second;
}
export function dock(
  root: DockNode,
  tab: DockTab,
  targetId: string,
  edge: DockEdge,
): DockNode {
  const target = groups(root).find((g) => g.id === targetId);
  if (!target) return root;
  // Moving a sole tab to the edge of its own group is a no-op, not an empty split.
  if (
    target.tabs.length === 1 &&
    target.tabs[0]?.id === tab.id &&
    edge !== "center"
  )
    return root;
  let next = JSON.parse(JSON.stringify(root)) as DockNode;
  for (const g of groups(next)) {
    g.tabs = g.tabs.filter((t) => t.id !== tab.id);
    if (!g.tabs.some((t) => t.id === g.active)) g.active = g.tabs[0]?.id || "";
  }
  const destination = groups(next).find((g) => g.id === targetId)!;
  if (edge === "center") {
    destination.tabs.push(tab);
    destination.active = tab.id;
  } else {
    const added = group([tab]);
    const before = edge === "left" || edge === "top";
    next = replaceNode(next, targetId, {
      kind: "split",
      id: uid(),
      axis: edge === "left" || edge === "right" ? "horizontal" : "vertical",
      ratio: 0.5,
      first: before ? added : destination,
      second: before ? destination : added,
    });
  }
  return prune(next) || group();
}
/** Reject malformed, oversized, or external restored layouts. */
export function validLayout(value: unknown): value is DockNode {
  const ids = new Set<string>();
  let count = 0;
  function check(v: any, depth: number): boolean {
    if (
      !v ||
      depth > 8 ||
      ++count > 60 ||
      typeof v.id !== "string" ||
      ids.has(v.id)
    )
      return false;
    ids.add(v.id);
    if (v.kind === "split")
      return (
        ["horizontal", "vertical"].includes(v.axis) &&
        Number.isFinite(v.ratio) &&
        v.ratio >= 0.15 &&
        v.ratio <= 0.85 &&
        check(v.first, depth + 1) &&
        check(v.second, depth + 1)
      );
    if (
      v.kind !== "group" ||
      !Array.isArray(v.tabs) ||
      v.tabs.length > 30 ||
      typeof v.active !== "string"
    )
      return false;
    return (
      v.tabs.every((t: any) => {
        if (
          !t ||
          typeof t.id !== "string" ||
          ids.has(t.id) ||
          typeof t.title !== "string" ||
          typeof t.path !== "string" ||
          !/^\/(?!\/)/.test(t.path)
        )
          return false;
        ids.add(t.id);
        return true;
      }) &&
      (!v.tabs.length || v.tabs.some((t: DockTab) => t.id === v.active))
    );
  }
  return check(value, 0);
}
