import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'Home', component: () => import('../views/HomeView.vue') },
  { path: '/exchange', name: 'CurrencyExchange', component: () => import('../views/CurrencyExchangeView.vue') },
  { path: '/news', name: 'News', component: () => import('../views/NewsView.vue') },
  { path: '/news/create', name: 'CreateArticle', component: () => import('../views/CreateArticleView.vue') },
  { path: '/news/:id', name: 'NewsDetail', component: () => import('../views/NewsDetailView.vue') },
  { path: '/news/:id/edit', name: 'EditArticle', component: () => import('../views/EditArticleView.vue') },
  { path: '/assistant', name: 'Assistant', component: () => import('../views/RAGAssistantView.vue') },
  { path: '/login', name: 'Login', component: () => import('../components/Login.vue') },
  { path: '/register', name: 'Register', component: () => import('../components/Register.vue') },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
