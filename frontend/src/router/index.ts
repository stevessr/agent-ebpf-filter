import { createRouter, createWebHistory } from 'vue-router';

const routes = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/dashboard/:tab?',
    name: 'Dashboard',
    component: () => import('../views/Dashboard.vue'),
  },
  {
    path: '/monitor/:tab?/:subtab?',
    name: 'Monitor',
    component: () => import('../views/Monitor.vue'),
  },
  {
    path: '/network',
    name: 'Network',
    component: () => import('../views/Network.vue'),
  },
  {
    path: '/network-flow/:tab?',
    name: 'NetworkFlow',
    component: () => import('../views/NetworkFlow.vue'),
  },
  {
    path: '/tls-capture',
    name: 'TLSCapture',
    component: () => import('../views/TLSCapture.vue'),
  },
  {
    path: '/execution-graph',
    name: 'ExecutionGraph',
    component: () => import('../views/ExecutionGraph.vue'),
  },
  {
    path: '/explorer',
    name: 'Explorer',
    component: () => import('../views/Explorer.vue'),
  },
  {
    path: '/executor/:tab?',
    name: 'Executor',
    component: () => import('../views/Executor.vue'),
  },
  {
    path: '/hooks',
    name: 'Hooks',
    component: () => import('../views/Hooks.vue'),
  },
  {
    path: '/ml/:subtab?',
    name: 'ML',
    component: () => import('../views/ML.vue'),
  },
  {
    path: '/plugins/:tab?',
    name: 'Plugins',
    component: () => import('../views/Plugins.vue'),
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
    component: () => import('../views/Config.vue'),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
