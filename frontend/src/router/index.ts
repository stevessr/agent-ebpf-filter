import { createRouter, createWebHistory } from 'vue-router';
import Dashboard from '../views/Dashboard.vue';
import Config from '../views/Config.vue';

const routes = [
  {
    path: '/',
    name: 'Dashboard',
    component: Dashboard,
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
