import { createRouter, createWebHistory } from 'vue-router';
import Dashboard from '../views/Dashboard.vue';
import Config from '../views/Config.vue';
import Monitor from '../views/Monitor.vue';
import Explorer from '../views/Explorer.vue';
import Executor from '../views/Executor.vue';
import Hooks from '../views/Hooks.vue';

const routes = [
  {
    path: '/',
    name: 'Dashboard',
    component: Dashboard,
  },
  {
    path: '/monitor',
    name: 'Monitor',
    component: Monitor,
  },
  {
    path: '/explorer',
    name: 'Explorer',
    component: Explorer,
  },
  {
    path: '/executor',
    name: 'Executor',
    component: Executor,
  },
  {
    path: '/hooks',
    name: 'Hooks',
    component: Hooks,
  },
  {
    path: '/config',
    name: 'Config',
    component: Config,
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
