import { ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { resolvePanelRoute } from "./resolvePanelRoute";
import { WORKBENCH_REGISTRY, resolveWorkbench } from "../../config/navigation";
import { dock, group, groups, prune, uid, validLayout } from "./layout";
import type { DockNode, DockTab, DockEdge } from "./layout";
const STORAGE_KEY = "agent.ide.layout.v1";
export function useDockWorkbench() {
  const router = useRouter(),
    route = useRoute();
  const makeTab = (path: string): DockTab => ({
    id: uid(),
    path,
    title: resolveWorkbench(resolvePanelRoute(router, path)).definition.title,
  });
  const storageAvailable = ref(true);
  const notice = ref("");
  function applyLayout(next: DockNode) {
    if (!validLayout(next) || groups(next).length > 12) {
      notice.value = "布局上限：12 个面板、8 层分屏、每面板 30 个标签";
      return false;
    }
    notice.value = "";
    root.value = next;
    return true;
  }
  function restore(): DockNode {
    try {
      const value = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
      if (validLayout(value) && groups(value).length <= 12) {
        for (const g of groups(value)) {
          g.tabs = g.tabs.filter(
            (t) => resolvePanelRoute(router, t.path).matched.length,
          );
          for (const tab of g.tabs)
            tab.path = resolvePanelRoute(router, tab.path).fullPath;
          if (!g.tabs.some((t) => t.id === g.active))
            g.active = g.tabs[0]?.id || "";
        }
        return prune(value) || group();
      }
    } catch {
      /* Storage may be disabled or from a newer schema. */
      try {
        localStorage.getItem(STORAGE_KEY);
      } catch {
        storageAvailable.value = false;
      }
    }
    return group();
  }
  const root = ref<DockNode>(restore());
  const focused = ref(groups(root.value)[0]!.id);
  const dragging = ref<string>("");
  function target(id = focused.value) {
    return (
      groups(root.value).find((g) => g.id === id) || groups(root.value)[0]!
    );
  }
  function syncURL() {
    const g = target(),
      tab = g.tabs.find((t) => t.id === g.active);
    if (tab && route.fullPath !== tab.path) void router.replace(tab.path);
  }
  function focus(id: string) {
    focused.value = id;
    syncURL();
  }
  function open(path: string, id = focused.value) {
    path = resolvePanelRoute(router, path).fullPath;
    const g = target(id),
      key = resolveWorkbench(resolvePanelRoute(router, path)).key;
    const existing = g.tabs.find(
      (t) => resolveWorkbench(resolvePanelRoute(router, t.path)).key === key,
    );
    if (existing) {
      existing.path = path;
      existing.title = makeTab(path).title;
      g.active = existing.id;
    } else {
      if (g.tabs.length >= 30) {
        notice.value = "当前面板已达到 30 个标签上限";
        return;
      }
      const tab = makeTab(path);
      g.tabs.push(tab);
      g.active = tab.id;
    }
    focused.value = g.id;
    syncURL();
  }
  function select(id: string, tab: string) {
    target(id).active = tab;
    focused.value = id;
    syncURL();
  }
  function close(id: string, tabId: string) {
    const g = target(id),
      index = g.tabs.findIndex((t) => t.id === tabId);
    g.tabs = g.tabs.filter((t) => t.id !== tabId);
    if (g.active === tabId) g.active = g.tabs[Math.max(0, index - 1)]?.id || "";
    root.value = prune(root.value) || group();
    if (!groups(root.value).some((g) => g.id === focused.value))
      focused.value = groups(root.value)[0]!.id;
    syncURL();
  }
  function drop(id: string, edge: DockEdge, event: DragEvent) {
    const token = event.dataTransfer?.getData("application/x-agent-panel");
    if (!token || token !== dragging.value) return;
    let tab = groups(root.value)
      .flatMap((g) => g.tabs)
      .find((t) => t.id === token);
    if (!tab && token.startsWith("nav:")) {
      const definition = Object.values(WORKBENCH_REGISTRY).find(
        (d) => `nav:${d.key}` === token,
      );
      if (definition)
        tab = makeTab(router.resolve(definition.defaultRoute).fullPath);
    }
    if (tab) {
      if (!applyLayout(dock(root.value, tab, id, edge))) {
        dragging.value = "";
        return;
      }
      focused.value = groups(root.value).find((g) =>
        g.tabs.some((t) => t.id === tab!.id),
      )!.id;
      syncURL();
    }
    dragging.value = "";
  }
  function drag(token: string, event: DragEvent) {
    dragging.value = token;
    event.dataTransfer?.setData("application/x-agent-panel", token);
    if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
  }
  function split(id: string, axis: "horizontal" | "vertical") {
    const g = target(id),
      tab = g.tabs.find((t) => t.id === g.active);
    if (!tab) return;
    applyLayout(
      dock(
        root.value,
        { ...tab, id: uid() },
        id,
        axis === "horizontal" ? "right" : "bottom",
      ),
    );
  }
  function resize(id: string, ratio: number) {
    function walk(n: DockNode) {
      if (n.kind === "split") {
        if (n.id === id) n.ratio = ratio;
        else {
          walk(n.first);
          walk(n.second);
        }
      }
    }
    walk(root.value);
  }
  function navigate(id: string, tabId: string, path: string) {
    const tab = target(id).tabs.find((t) => t.id === tabId);
    if (tab) {
      tab.path = path;
      tab.title = makeTab(path).title;
      if (id === focused.value) syncURL();
    }
  }
  function reset() {
    root.value = group([makeTab(route.fullPath)]);
    focused.value = root.value.id;
  }
  watch(
    () => route.fullPath,
    (path) => {
      if (!route.matched.length) return;
      const all = groups(root.value),
        current = target();
      if (current.tabs.some((t) => t.id === current.active && t.path === path))
        return;
      const found = all.find((g) => g.tabs.some((t) => t.path === path));
      if (found) {
        focused.value = found.id;
        found.active = found.tabs.find((t) => t.path === path)!.id;
      } else open(path);
    },
    { immediate: true },
  );
  watch(
    root,
    (value) => {
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
      } catch {
        /* Non-persistent mode remains usable. */
        storageAvailable.value = false;
      }
    },
    { deep: true },
  );
  return {
    notice,
    storageAvailable,
    root,
    focused,
    dragging,
    open,
    focus,
    select,
    close,
    drag,
    drop,
    split,
    resize,
    navigate,
    reset,
  };
}
