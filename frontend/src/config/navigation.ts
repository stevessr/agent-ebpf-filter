import {
  AppstoreOutlined,
  BarChartOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  FolderOpenOutlined,
  GlobalOutlined,
  LinkOutlined,
  PlaySquareOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons-vue';
import type { RouteLocationNormalizedLoaded, RouteLocationRaw } from 'vue-router';
import type {
  NavMenuGroup,
  ResolvedWorkbench,
  WorkbenchDefinition,
  WorkbenchKey,
  WorkbenchTabState,
} from '../types/navigation';

export const NAV_GROUPS: NavMenuGroup[] = [
  {
    key: 'observability',
    title: '观测分析',
    children: [
      { key: 'dashboard', title: 'Dashboard', icon: DashboardOutlined, defaultRoute: { name: 'Dashboard' }, affix: true, closable: false },
      { key: 'monitor', title: 'Monitor', icon: BarChartOutlined, defaultRoute: { name: 'Monitor', params: { tab: 'dashboard', subtab: 'cpu' } } },
      { key: 'network', title: 'Network', icon: GlobalOutlined, defaultRoute: { name: 'Network' } },
      { key: 'network-flow', title: 'Traffic', icon: DeploymentUnitOutlined, defaultRoute: { name: 'NetworkFlow', params: { tab: 'overview' } } },
      { key: 'execution-graph', title: 'Tracing', icon: ClusterOutlined, defaultRoute: { name: 'ExecutionGraph', params: { tab: 'topology' } } },
      { key: 'tls-capture', title: 'Hook SSL', icon: SafetyCertificateOutlined, defaultRoute: { name: 'TLSCapture' } },
    ],
  },
  {
    key: 'tools',
    title: '执行与工具',
    children: [
      { key: 'explorer', title: 'Explorer', icon: FolderOpenOutlined, defaultRoute: { name: 'Explorer' } },
      { key: 'executor', title: 'Executor', icon: PlaySquareOutlined, defaultRoute: { name: 'Executor', params: { tab: 'shell' } } },
      { key: 'hooks', title: 'Hooks', icon: LinkOutlined, defaultRoute: { name: 'Hooks' } },
    ],
  },
  {
    key: 'intelligence',
    title: '智能与扩展',
    children: [
      { key: 'ml', title: 'ML', icon: ThunderboltOutlined, defaultRoute: { name: 'ML', params: { subtab: 'status' } } },
      { key: 'plugins', title: '插件', icon: AppstoreOutlined, defaultRoute: { name: 'Plugins', params: { tab: 'list' } } },
    ],
  },
  {
    key: 'system',
    title: '系统配置',
    children: [
      { key: 'config', title: 'Configuration', icon: SettingOutlined, defaultRoute: { name: 'Config', params: { tab: 'registry' } } },
    ],
  },
];

export const WORKBENCH_REGISTRY = NAV_GROUPS.reduce<Record<WorkbenchKey, WorkbenchDefinition>>((registry, group) => {
  for (const child of group.children) {
    registry[child.key] = {
      ...child,
      groupKey: group.key,
    };
  }
  return registry;
}, {} as Record<WorkbenchKey, WorkbenchDefinition>);

const routeNameMap: Record<string, WorkbenchKey> = {
  Dashboard: 'dashboard',
  Monitor: 'monitor',
  Network: 'network',
  NetworkFlow: 'network-flow',
  TLSCapture: 'tls-capture',
  ExecutionGraph: 'execution-graph',
  Explorer: 'explorer',
  Executor: 'executor',
  Hooks: 'hooks',
  ML: 'ml',
  Plugins: 'plugins',
  Config: 'config',
};

export const getWorkbenchDefaultRoute = (key: WorkbenchKey): RouteLocationRaw => WORKBENCH_REGISTRY[key].defaultRoute;

export const resolveWorkbenchKey = (route: RouteLocationNormalizedLoaded): WorkbenchKey => {
  const routeName = typeof route.name === 'string' ? route.name : '';
  const mapped = routeNameMap[routeName];
  if (mapped) return mapped;

  const path = route.path;
  if (path.startsWith('/agentsight') || path.startsWith('/execution-graph')) return 'execution-graph';
  if (path.startsWith('/config/ml') || path.startsWith('/ml')) return 'ml';
  if (path.startsWith('/dashboard')) return 'dashboard';
  if (path.startsWith('/monitor')) return 'monitor';
  if (path.startsWith('/network-flow')) return 'network-flow';
  if (path.startsWith('/network')) return 'network';
  if (path.startsWith('/tls-capture')) return 'tls-capture';
  if (path.startsWith('/explorer')) return 'explorer';
  if (path.startsWith('/executor')) return 'executor';
  if (path.startsWith('/hooks')) return 'hooks';
  if (path.startsWith('/plugins')) return 'plugins';
  if (path.startsWith('/config')) return 'config';
  return 'dashboard';
};

export const resolveWorkbench = (route: RouteLocationNormalizedLoaded): ResolvedWorkbench => {
  const key = resolveWorkbenchKey(route);
  return { key, definition: WORKBENCH_REGISTRY[key] };
};

export const createRouteSnapshot = (route: RouteLocationNormalizedLoaded): RouteLocationRaw => ({
  name: route.name ?? undefined,
  params: { ...route.params },
  query: { ...route.query },
  hash: route.hash,
});

export const createWorkbenchTab = (key: WorkbenchKey, lastRoute?: RouteLocationRaw): WorkbenchTabState => {
  const definition = WORKBENCH_REGISTRY[key];
  return {
    key,
    title: definition.title,
    menuKey: key,
    groupKey: definition.groupKey,
    lastRoute: lastRoute ?? definition.defaultRoute,
    closable: definition.closable ?? !definition.affix,
    affix: definition.affix ?? false,
  };
};

export const getAffixWorkbenchTabs = (): WorkbenchTabState[] => NAV_GROUPS.flatMap((group) =>
  group.children
    .filter((child) => child.affix)
    .map((child) => createWorkbenchTab(child.key)),
);
