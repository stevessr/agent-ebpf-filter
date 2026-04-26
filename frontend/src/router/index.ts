import { createRouter, createWebHistory } from 'vue-router';

const routes = [
  {
    path: '/',
    name: 'Dashboard',
    component: () => import('../views/Dashboard.vue'),
  },
  {
    path: '/monitor',
    name: 'Monitor',
    component: () => import('../views/Monitor.vue'),
  },
  {
    path: '/network',
    name: 'Network',
    component: () => import('../views/Network.vue'),
  },
  {
    path: '/network-flow',
    name: 'NetworkFlow',
    component: () => import('../views/NetworkFlow.vue'),
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
    path: '/config/:tab?/:subtab?',
    name: 'Config',
    component: () => import('../views/Config.vue'),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
