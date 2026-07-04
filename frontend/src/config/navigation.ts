import {
  AppstoreOutlined,
  BarChartOutlined,
  BulbOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  ExperimentOutlined,
  EyeOutlined,
  FolderOpenOutlined,
  GlobalOutlined,
  LinkOutlined,
  PlaySquareOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from "@ant-design/icons-vue";
import type {
  RouteLocationNormalizedLoaded,
  RouteLocationRaw,
} from "vue-router";
import { filterFeaturesForFrontendBuild } from "./featureFlags";
import type {
  NavMenuGroup,
  ResolvedWorkbench,
  WorkbenchDefinition,
  WorkbenchKey,
  WorkbenchTabState,
} from "../types/navigation";

const ALL_NAV_GROUPS: NavMenuGroup[] = [
  {
    key: "observability",
    title: "观测分析",
    icon: EyeOutlined,
    children: [
      {
        key: "dashboard",
        title: "Dashboard",
        icon: DashboardOutlined,
        defaultRoute: { name: "Dashboard" },
        affix: true,
        closable: false,
      },
      {
        key: "monitor",
        title: "Monitor",
        icon: BarChartOutlined,
        defaultRoute: {
          name: "Monitor",
          params: { tab: "dashboard", subtab: "cpu" },
        },
      },
      {
        key: "network",
        title: "Network",
        icon: GlobalOutlined,
        defaultRoute: { name: "Network" },
      },
      {
        key: "network-flow",
        title: "Traffic",
        icon: DeploymentUnitOutlined,
        defaultRoute: { name: "NetworkFlow", params: { tab: "overview" } },
      },
      {
        key: "execution-graph",
        title: "Tracing",
        icon: ClusterOutlined,
        defaultRoute: { name: "ExecutionGraph", params: { tab: "topology" } },
      },
      {
        key: "research",
        title: "Research",
        icon: ExperimentOutlined,
        defaultRoute: { name: "Research" },
      },
      {
        key: "tls-capture",
        title: "Hook SSL",
        icon: SafetyCertificateOutlined,
        defaultRoute: { name: "TLSCapture" },
        feature: "tls_capture",
      },
      {
        key: "observe",
        title: "观察分析",
        icon: RadarChartOutlined,
        defaultRoute: { name: "Observe" },
      },
    ],
  },
  {
    key: "tools",
    title: "执行与工具",
    icon: ToolOutlined,
    children: [
      {
        key: "explorer",
        title: "Explorer",
        icon: FolderOpenOutlined,
        defaultRoute: { name: "Explorer" },
      },
      {
        key: "executor",
        title: "Executor",
        icon: PlaySquareOutlined,
        defaultRoute: { name: "Executor", params: { tab: "shell" } },
        feature: "shell_sessions",
      },
      {
        key: "hooks",
        title: "Hooks",
        icon: LinkOutlined,
        defaultRoute: { name: "Hooks" },
        feature: "hooks",
      },
    ],
  },
  {
    key: "intelligence",
    title: "智能与扩展",
    icon: BulbOutlined,
    children: [
      {
        key: "ml",
        title: "ML",
        icon: ThunderboltOutlined,
        defaultRoute: { name: "ML", params: { subtab: "status" } },
        feature: "ml",
      },
      {
        key: "plugins",
        title: "插件",
        icon: AppstoreOutlined,
        defaultRoute: { name: "Plugins", params: { tab: "list" } },
        feature: "plugins",
      },
    ],
  },
  {
    key: "system",
    title: "系统配置",
    icon: SettingOutlined,
    children: [
      {
        key: "config",
        title: "Configuration",
        icon: SettingOutlined,
        defaultRoute: { name: "Config", params: { tab: "registry" } },
      },
    ],
  },
];

export const NAV_GROUPS: NavMenuGroup[] = ALL_NAV_GROUPS.map((group) => ({
  ...group,
  children: filterFeaturesForFrontendBuild(group.children),
})).filter((group) => group.children.length > 0);

export const WORKBENCH_REGISTRY = NAV_GROUPS.reduce<
  Record<WorkbenchKey, WorkbenchDefinition>
>(
  (registry, group) => {
    for (const child of group.children) {
      registry[child.key] = {
        ...child,
        groupKey: group.key,
      };
    }
    return registry;
  },
  {} as Record<WorkbenchKey, WorkbenchDefinition>,
);

const routeNameMap: Record<string, WorkbenchKey> = {
  Dashboard: "dashboard",
  Monitor: "monitor",
  Network: "network",
  NetworkFlow: "network-flow",
  TLSCapture: "tls-capture",
  ExecutionGraph: "execution-graph",
  Research: "research",
  Explorer: "explorer",
  Executor: "executor",
  Hooks: "hooks",
  ML: "ml",
  Plugins: "plugins",
  Config: "config",
  Observe: "observe",
};

const isAvailableWorkbenchKey = (key: WorkbenchKey) =>
  key in WORKBENCH_REGISTRY;

const fallbackWorkbenchKey = (): WorkbenchKey =>
  isAvailableWorkbenchKey("dashboard")
    ? "dashboard"
    : (Object.keys(WORKBENCH_REGISTRY)[0] as WorkbenchKey);

export const getWorkbenchDefaultRoute = (key: WorkbenchKey): RouteLocationRaw =>
  WORKBENCH_REGISTRY[key]?.defaultRoute ||
  WORKBENCH_REGISTRY[fallbackWorkbenchKey()].defaultRoute;

export const resolveWorkbenchKey = (
  route: RouteLocationNormalizedLoaded,
): WorkbenchKey => {
  const routeName = typeof route.name === "string" ? route.name : "";
  const mapped = routeNameMap[routeName];
  if (mapped && isAvailableWorkbenchKey(mapped)) return mapped;

  const path = route.path;
  if (
    (path.startsWith("/agentsight") || path.startsWith("/execution-graph")) &&
    isAvailableWorkbenchKey("execution-graph")
  )
    return "execution-graph";
  if (
    (path.startsWith("/config/ml") || path.startsWith("/ml")) &&
    isAvailableWorkbenchKey("ml")
  )
    return "ml";
  if (path.startsWith("/dashboard") && isAvailableWorkbenchKey("dashboard"))
    return "dashboard";
  if (path.startsWith("/monitor") && isAvailableWorkbenchKey("monitor"))
    return "monitor";
  if (path.startsWith("/research") && isAvailableWorkbenchKey("research"))
    return "research";
  if (
    path.startsWith("/network-flow") &&
    isAvailableWorkbenchKey("network-flow")
  )
    return "network-flow";
  if (path.startsWith("/network") && isAvailableWorkbenchKey("network"))
    return "network";
  if (path.startsWith("/tls-capture") && isAvailableWorkbenchKey("tls-capture"))
    return "tls-capture";
  if (path.startsWith("/explorer") && isAvailableWorkbenchKey("explorer"))
    return "explorer";
  if (path.startsWith("/executor") && isAvailableWorkbenchKey("executor"))
    return "executor";
  if (path.startsWith("/hooks") && isAvailableWorkbenchKey("hooks"))
    return "hooks";
  if (path.startsWith("/plugins") && isAvailableWorkbenchKey("plugins"))
    return "plugins";
  if (path.startsWith("/config") && isAvailableWorkbenchKey("config"))
    return "config";
  if (path.startsWith("/observe") && isAvailableWorkbenchKey("observe"))
    return "observe";
  return fallbackWorkbenchKey();
};

export const resolveWorkbench = (
  route: RouteLocationNormalizedLoaded,
): ResolvedWorkbench => {
  const key = resolveWorkbenchKey(route);
  return { key, definition: WORKBENCH_REGISTRY[key] };
};

export const createRouteSnapshot = (
  route: RouteLocationNormalizedLoaded,
): RouteLocationRaw => ({
  name: route.name ?? undefined,
  params: { ...route.params },
  query: { ...route.query },
  hash: route.hash,
});

export const createWorkbenchTab = (
  key: WorkbenchKey,
  lastRoute?: RouteLocationRaw,
): WorkbenchTabState => {
  const definition =
    WORKBENCH_REGISTRY[key] || WORKBENCH_REGISTRY[fallbackWorkbenchKey()];
  return {
    key: definition.key,
    title: definition.title,
    menuKey: definition.key,
    groupKey: definition.groupKey,
    lastRoute: lastRoute ?? definition.defaultRoute,
    closable: definition.closable ?? !definition.affix,
    affix: definition.affix ?? false,
  };
};

export const getAffixWorkbenchTabs = (): WorkbenchTabState[] =>
  NAV_GROUPS.flatMap((group) =>
    group.children
      .filter((child) => child.affix)
      .map((child) => createWorkbenchTab(child.key)),
  );
