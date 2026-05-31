import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { RouteLocationRaw } from "vue-router";
import {
  NAV_GROUPS,
  WORKBENCH_REGISTRY,
  createRouteSnapshot,
  createWorkbenchTab,
  getAffixWorkbenchTabs,
  getWorkbenchDefaultRoute,
  resolveWorkbench,
} from "../../config/navigation";
import type { WorkbenchKey, WorkbenchTabState } from "../../types/navigation";

const STORAGE_KEY = "app.workbenchTabs.v1";
const COLLAPSED_STORAGE_KEY = "app.sideNav.collapsed.v1";

type PersistedWorkbenchState = {
  tabs?: WorkbenchTabState[];
};

const isWorkbenchKey = (key: unknown): key is WorkbenchKey =>
  typeof key === "string" && key in WORKBENCH_REGISTRY;

const readStoredTabs = (): WorkbenchTabState[] => {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as PersistedWorkbenchState;
    if (!Array.isArray(parsed.tabs)) return [];
    return parsed.tabs
      .filter((tab) => isWorkbenchKey(tab.key))
      .map((tab) => ({
        ...createWorkbenchTab(
          tab.key,
          tab.lastRoute || getWorkbenchDefaultRoute(tab.key),
        ),
        lastRoute: tab.lastRoute || getWorkbenchDefaultRoute(tab.key),
      }));
  } catch {
    return [];
  }
};

const readStoredCollapsed = () => {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(COLLAPSED_STORAGE_KEY) === "true";
};

const writeStoredTabs = (tabs: WorkbenchTabState[]) => {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ tabs }));
};

const writeStoredCollapsed = (collapsed: boolean) => {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(
    COLLAPSED_STORAGE_KEY,
    collapsed ? "true" : "false",
  );
};

const uniqueTabs = (tabs: WorkbenchTabState[]) => {
  const seen = new Set<WorkbenchKey>();
  return tabs.filter((tab) => {
    if (seen.has(tab.key)) return false;
    seen.add(tab.key);
    return true;
  });
};

export function useWorkbenchNavigation() {
  const route = useRoute();
  const router = useRouter();

  const collapsed = ref(readStoredCollapsed());
  const userOpenKeys = ref<string[]>(NAV_GROUPS.map((group) => group.key));
  const openedTabs = ref<WorkbenchTabState[]>(
    uniqueTabs([...getAffixWorkbenchTabs(), ...readStoredTabs()]),
  );

  const activeWorkbench = computed(() => resolveWorkbench(route));
  const activeTabKey = computed(() => activeWorkbench.value.key);
  const selectedMenuKeys = computed(() => [activeWorkbench.value.key]);
  const routeOpenKeys = computed(() => [
    activeWorkbench.value.definition.groupKey,
  ]);
  const openKeys = computed(() =>
    Array.from(new Set([...userOpenKeys.value, ...routeOpenKeys.value])),
  );

  const ensureAffixTabs = () => {
    const affixTabs = getAffixWorkbenchTabs();
    const existingKeys = new Set(openedTabs.value.map((tab) => tab.key));
    const missing = affixTabs.filter((tab) => !existingKeys.has(tab.key));
    if (missing.length) {
      openedTabs.value = uniqueTabs([...missing, ...openedTabs.value]);
    }
  };

  const upsertTab = (key: WorkbenchKey, lastRoute: RouteLocationRaw) => {
    const existing = openedTabs.value.find((tab) => tab.key === key);
    if (existing) {
      openedTabs.value = openedTabs.value.map((tab) =>
        tab.key === key ? { ...tab, lastRoute } : tab,
      );
      return;
    }
    openedTabs.value = [
      ...openedTabs.value,
      createWorkbenchTab(key, lastRoute),
    ];
  };

  const navigateToRoute = (target: RouteLocationRaw) => {
    void router.push(target);
  };

  const handleMenuSelect = ({ key }: { key: string }) => {
    if (!isWorkbenchKey(key)) return;
    const existing = openedTabs.value.find((tab) => tab.key === key);
    navigateToRoute(existing?.lastRoute || getWorkbenchDefaultRoute(key));
  };

  const handleTabChange = (key: string | number) => {
    if (!isWorkbenchKey(key)) return;
    const tab = openedTabs.value.find((item) => item.key === key);
    navigateToRoute(tab?.lastRoute || getWorkbenchDefaultRoute(key));
  };

  const closeTab = (key: WorkbenchKey) => {
    const tab = openedTabs.value.find((item) => item.key === key);
    if (!tab || !tab.closable) return;

    const currentIndex = openedTabs.value.findIndex((item) => item.key === key);
    const isActive = activeTabKey.value === key;
    const nextTabs = openedTabs.value.filter((item) => item.key !== key);
    openedTabs.value = nextTabs;
    ensureAffixTabs();

    if (!isActive) return;

    const fallback =
      nextTabs[currentIndex] ||
      nextTabs[currentIndex - 1] ||
      openedTabs.value[0] ||
      createWorkbenchTab("dashboard");
    navigateToRoute(
      fallback.lastRoute || getWorkbenchDefaultRoute(fallback.key),
    );
  };

  const handleTabEdit = (
    targetKey: string | number | MouseEvent,
    action: "add" | "remove",
  ) => {
    if (action !== "remove" || !isWorkbenchKey(targetKey)) return;
    closeTab(targetKey);
  };

  const updateOpenKeys = (keys: string[]) => {
    userOpenKeys.value = keys;
  };

  watch(
    () => route.fullPath,
    () => {
      const { key } = activeWorkbench.value;
      upsertTab(key, createRouteSnapshot(route));
    },
    { immediate: true },
  );

  watch(
    openedTabs,
    (tabs) => {
      ensureAffixTabs();
      writeStoredTabs(tabs);
    },
    { deep: true },
  );

  watch(collapsed, writeStoredCollapsed);

  return {
    navGroups: NAV_GROUPS,
    collapsed,
    openedTabs,
    activeTabKey,
    selectedMenuKeys,
    openKeys,
    handleMenuSelect,
    handleTabChange,
    handleTabEdit,
    updateOpenKeys,
  };
}
