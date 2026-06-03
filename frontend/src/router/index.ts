import { createRouter, createWebHistory } from "vue-router";
import type { RouteLocationNormalized } from "vue-router";
import { isFeatureIncludedInFrontendBuild } from "../config/featureFlags";
import type { FeatureID } from "../types/feature";

const featureMeta = (feature: FeatureID) => ({ feature });

const routeFeature = (
  route: RouteLocationNormalized,
): FeatureID | undefined => {
  const feature = route.meta.feature;
  return typeof feature === "string" ? (feature as FeatureID) : undefined;
};

const routes = [
  {
    path: "/",
    redirect: "/dashboard",
  },
  {
    path: "/dashboard/:tab?",
    name: "Dashboard",
    component: () => import("../views/dashboard/Dashboard.vue"),
  },
  {
    path: "/monitor/:tab?/:subtab?",
    name: "Monitor",
    component: () => import("../views/monitor/Monitor.vue"),
  },
  {
    path: "/network",
    name: "Network",
    component: () => import("../views/network/Network.vue"),
  },
  {
    path: "/network-flow/:tab?",
    name: "NetworkFlow",
    component: () => import("../views/network/NetworkFlow.vue"),
  },
  {
    path: "/tls-capture",
    name: "TLSCapture",
    meta: featureMeta("tls_capture"),
    component: () => import("../views/network/TLSCapture.vue"),
  },
  {
    path: "/agentsight/:tab?",
    redirect: { name: "ExecutionGraph", params: { tab: "behavior" } },
  },
  {
    path: "/execution-graph/:tab?",
    name: "ExecutionGraph",
    component: () => import("../views/execution-graph/ExecutionGraph.vue"),
  },
  {
    path: "/explorer",
    name: "Explorer",
    component: () => import("../views/explorer/Explorer.vue"),
  },
  {
    path: "/executor/:tab?/:subtab?",
    name: "Executor",
    meta: featureMeta("shell_sessions"),
    component: () => import("../views/executor/Executor.vue"),
  },
  {
    path: "/hooks",
    name: "Hooks",
    meta: featureMeta("hooks"),
    component: () => import("../views/hooks/Hooks.vue"),
  },
  {
    path: "/ml/:subtab?",
    name: "ML",
    meta: featureMeta("ml"),
    component: () => import("../views/ml/ML.vue"),
  },
  {
    path: "/plugins/:tab?",
    name: "Plugins",
    meta: featureMeta("plugins"),
    component: () => import("../views/plugins/Plugins.vue"),
  },
  {
    path: "/config/ml/:subtab?",
    redirect: (to: { params: { subtab?: string | string[] } }) => {
      const subtab = Array.isArray(to.params.subtab)
        ? to.params.subtab[0]
        : to.params.subtab;
      return subtab ? { name: "ML", params: { subtab } } : { name: "ML" };
    },
  },
  {
    path: "/config/:tab?/:subtab?/:subsubtab?",
    name: "Config",
    component: () => import("../views/config/Config.vue"),
  },
  {
    path: "/feature-unavailable",
    name: "FeatureUnavailable",
    component: () => import("../views/config/FeatureUnavailable.vue"),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach((to) => {
  const feature = routeFeature(to);
  if (!feature || isFeatureIncludedInFrontendBuild(feature)) return true;
  return {
    name: "FeatureUnavailable",
    query: { feature, from: to.fullPath },
  };
});

export default router;
