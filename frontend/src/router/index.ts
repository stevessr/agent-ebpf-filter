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
