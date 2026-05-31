import { createRouter, createWebHistory } from 'vue-router';

const routes = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/dashboard/:tab?',
    name: 'Dashboard',
    component: () => import('../views/dashboard/Dashboard.vue'),
  },
  {
    path: '/monitor/:tab?/:subtab?',
    name: 'Monitor',
    component: () => import('../views/monitor/Monitor.vue'),
  },
  {
    path: '/network',
    name: 'Network',
    component: () => import('../views/network/Network.vue'),
  },
  {
    path: '/network-flow/:tab?',
    name: 'NetworkFlow',
    component: () => import('../views/network/NetworkFlow.vue'),
  },
  {
    path: '/tls-capture',
    name: 'TLSCapture',
    component: () => import('../views/network/TLSCapture.vue'),
  },
  {
    path: '/agentsight/:tab?',
    redirect: { name: 'ExecutionGraph', params: { tab: 'behavior' } },
  },
  {
    path: '/execution-graph/:tab?',
    name: 'ExecutionGraph',
    component: () => import('../views/execution-graph/ExecutionGraph.vue'),
  },
  {
    path: '/explorer',
    name: 'Explorer',
    component: () => import('../views/explorer/Explorer.vue'),
  },
  {
    path: '/executor/:tab?/:subtab?',
    name: 'Executor',
    component: () => import('../views/executor/Executor.vue'),
  },
  {
    path: '/hooks',
    name: 'Hooks',
    component: () => import('../views/hooks/Hooks.vue'),
  },
  {
    path: '/ml/:subtab?',
    name: 'ML',
    component: () => import('../views/ml/ML.vue'),
  },
  {
    path: '/plugins/:tab?',
    name: 'Plugins',
    component: () => import('../views/plugins/Plugins.vue'),
  },
  {
    path: '/config/ml/:subtab?',
    redirect: (to: { params: { subtab?: string | string[] } }) => {
      const subtab = Array.isArray(to.params.subtab) ? to.params.subtab[0] : to.params.subtab;
      return subtab ? { name: 'ML', params: { subtab } } : { name: 'ML' };
    },
  },
  {
    path: '/config/:tab?/:subtab?/:subsubtab?',
    name: 'Config',
    component: () => import('../views/config/Config.vue'),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
