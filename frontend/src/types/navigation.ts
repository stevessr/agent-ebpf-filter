import type { Component } from "vue";
import type { FeatureID } from "./feature";
import type {
  RouteLocationNormalizedLoaded,
  RouteLocationRaw,
} from "vue-router";

export type WorkbenchKey =
  | "dashboard"
  | "monitor"
  | "network"
  | "network-flow"
  | "tls-capture"
  | "execution-graph"
  | "research"
  | "explorer"
  | "executor"
  | "hooks"
  | "ml"
  | "plugins"
  | "config"
  | "signals"
  | "observe";

export type NavMenuLeaf = {
  key: WorkbenchKey;
  title: string;
  icon: Component;
  defaultRoute: RouteLocationRaw;
  closable?: boolean;
  affix?: boolean;
  feature?: FeatureID;
};

export type NavMenuGroup = {
  key: string;
  title: string;
  icon: Component;
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

export type WorkbenchRouteResolver = (
  route: RouteLocationNormalizedLoaded,
) => WorkbenchKey | null;
