import type { Component } from 'vue';
import type { RouteLocationNormalizedLoaded, RouteLocationRaw } from 'vue-router';

export type WorkbenchKey =
  | 'dashboard'
  | 'monitor'
  | 'network'
  | 'network-flow'
  | 'tls-capture'
  | 'execution-graph'
  | 'explorer'
  | 'executor'
  | 'hooks'
  | 'ml'
  | 'plugins'
  | 'config';

export type NavMenuLeaf = {
  key: WorkbenchKey;
  title: string;
  icon: Component;
  defaultRoute: RouteLocationRaw;
  closable?: boolean;
  affix?: boolean;
};

export type NavMenuGroup = {
  key: string;
  title: string;
  children: NavMenuLeaf[];
};

export type WorkbenchDefinition = NavMenuLeaf & {
  groupKey: string;
};

export type WorkbenchTabState = {
  key: WorkbenchKey;
  title: string;
  menuKey: WorkbenchKey;
  groupKey: string;
  lastRoute: RouteLocationRaw;
  closable: boolean;
  affix: boolean;
};

export type ResolvedWorkbench = {
  key: WorkbenchKey;
  definition: WorkbenchDefinition;
};

export type WorkbenchRouteResolver = (route: RouteLocationNormalizedLoaded) => WorkbenchKey | null;
