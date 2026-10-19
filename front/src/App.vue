<template>
  <el-container class="layout-shell">
    <el-header class="header-shell">
      <div class="header-inner">
        <div class="brand" @click="router.push({ name: 'Home' })">
          <div class="brand-icon">
            <svg viewBox="0 0 32 32" width="28" height="28">
              <defs>
                <linearGradient id="brandGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" style="stop-color:#22d3ee"/>
                  <stop offset="50%" style="stop-color:#5b9cf5"/>
                  <stop offset="100%" style="stop-color:#a78bfa"/>
                </linearGradient>
              </defs>
              <circle cx="16" cy="16" r="14" fill="none" stroke="url(#brandGrad)" stroke-width="2.5"/>
              <path d="M10 18 L14 10 L18 22 L22 14" fill="none" stroke="url(#brandGrad)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </div>
          <span class="brand-text">星汇论坛</span>
        </div>
        <el-menu
          :default-active="activeIndex"
          class="top-menu"
          mode="horizontal"
          :ellipsis="false"
          @select="handleSelect"
        >
          <el-menu-item index="Home">
            <span class="menu-icon">✦</span> 首页
          </el-menu-item>
          <el-menu-item index="CurrencyExchange">
            <span class="menu-icon">◎</span> 兑换货币
          </el-menu-item>
          <el-menu-item index="News">
            <span class="menu-icon">◈</span> 查看新闻
          </el-menu-item>
          <el-menu-item index="CreateArticle" v-if="authStore.isAuthenticated">
            <span class="menu-icon">✦</span> 发布文章
          </el-menu-item>
          <el-menu-item index="Assistant" v-if="authStore.isAuthenticated">
            <span class="menu-icon">◉</span> RAG 助手
          </el-menu-item>
          <el-menu-item index="Login" v-if="!authStore.isAuthenticated">
            <span class="menu-icon">◇</span> 登录
          </el-menu-item>
          <el-menu-item index="Register" v-if="!authStore.isAuthenticated">
            <span class="menu-icon">◆</span> 注册
          </el-menu-item>
          <el-menu-item index="logout" v-if="authStore.isAuthenticated" class="logout-item">
            <span class="menu-icon">◈</span> 退出
          </el-menu-item>
        </el-menu>
        <button class="mobile-menu-toggle" type="button" :aria-expanded="mobileMenuOpen" aria-label="打开导航菜单" @click="mobileMenuOpen = !mobileMenuOpen">☰</button>
        <nav v-if="mobileMenuOpen" class="mobile-nav" aria-label="移动端导航">
          <button v-for="item in mobileNavItems" :key="item.key" type="button" :class="{ active: activeIndex === item.key }" @click="handleSelect(item.key)">{{ item.label }}</button>
        </nav>
      </div>
    </el-header>
    <el-main class="main-shell">
      <router-view></router-view>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAuthStore } from './store/auth';

const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();
const activeIndex = ref(route.name?.toString() || 'Home');
const mobileMenuOpen = ref(false);
const mobileNavItems = computed(() => [
  { key: 'Home', label: '首页' },
  { key: 'CurrencyExchange', label: '兑换货币' },
  { key: 'News', label: '查看新闻' },
  ...(authStore.isAuthenticated
    ? [{ key: 'CreateArticle', label: '发布文章' }, { key: 'Assistant', label: 'RAG 助手' }, { key: 'logout', label: '退出登录' }]
    : [{ key: 'Login', label: '登录' }, { key: 'Register', label: '注册' }]),
]);

watch(route, (newRoute) => {
  activeIndex.value = newRoute.name?.toString() || 'Home';
  mobileMenuOpen.value = false;
});

const handleSelect = async (key: string) => {
  mobileMenuOpen.value = false;
  if (key === 'logout') {
    await authStore.logout();
    router.push({ name: 'Home' });
  } else {
    router.push({ name: key });
  }
};
</script>

<style scoped>
.layout-shell {
  min-height: 100vh;
}

.header-shell {
  height: 76px;
  padding: 12px 18px;
  background: transparent;
  position: sticky;
  top: 0;
  z-index: 100;
}

.header-inner {
  height: 100%;
  max-width: 1160px;
  margin: 0 auto;
  padding: 0 20px;
  border-radius: 20px;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  background: rgba(10, 20, 48, 0.7);
  border: 1px solid rgba(100, 160, 255, 0.15);
  box-shadow: 0 4px 30px rgba(0, 0, 0, 0.3), 0 0 30px rgba(91, 156, 245, 0.08);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  flex-shrink: 0;
}

.brand-icon {
  display: flex;
  align-items: center;
  animation: brandPulse 3s ease-in-out infinite;
}

@keyframes brandPulse {
  0%, 100% { filter: drop-shadow(0 0 6px rgba(34, 211, 238, 0.4)); }
  50% { filter: drop-shadow(0 0 16px rgba(167, 139, 250, 0.6)); }
}

.brand-text {
  font-size: 19px;
  font-weight: 800;
  background: linear-gradient(135deg, var(--accent-cyan), var(--accent-blue), var(--accent-purple));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: 1px;
}

.top-menu {
  border-bottom: none !important;
  background: transparent !important;
}

.top-menu :deep(.el-menu-item) {
  border-radius: 10px;
  margin: 0 2px;
  height: 40px;
  line-height: 40px;
  font-size: 14px;
  color: var(--text-secondary) !important;
  transition: all 0.3s ease;
}

.top-menu :deep(.el-menu-item:hover) {
  color: var(--accent-cyan) !important;
  background: rgba(100, 160, 255, 0.08) !important;
}

.top-menu :deep(.el-menu-item.is-active) {
  color: var(--accent-blue) !important;
  background: rgba(91, 156, 245, 0.12) !important;
  box-shadow: inset 0 0 0 1px rgba(91, 156, 245, 0.2);
}

.menu-icon {
  font-size: 10px;
  margin-right: 4px;
  opacity: 0.6;
}

.logout-item :deep(.el-menu-item) {
  color: rgba(248, 113, 113, 0.7) !important;
}

.logout-item :deep(.el-menu-item:hover) {
  color: #f87171 !important;
  background: rgba(248, 113, 113, 0.08) !important;
}

.main-shell {
  padding: 0;
}

.mobile-menu-toggle, .mobile-nav { display: none; }

@media (max-width: 900px) {
  .header-shell { height: 68px; padding: 8px 12px; }
  .header-inner { padding: 0 16px; border-radius: 16px; position: relative; }
  .top-menu { display: none !important; }
  .mobile-menu-toggle {
    display: block;
    border: 1px solid rgba(100, 160, 255, 0.3);
    color: var(--text-primary);
    background: rgba(100, 160, 255, 0.1);
    border-radius: 10px;
    font-size: 22px;
    width: 44px;
    height: 40px;
    cursor: pointer;
  }
  .mobile-nav {
    display: flex;
    position: absolute;
    top: calc(100% + 8px);
    left: 0;
    right: 0;
    flex-direction: column;
    gap: 4px;
    padding: 10px;
    border: 1px solid rgba(100, 160, 255, 0.25);
    border-radius: 14px;
    background: #0d1838;
    box-shadow: 0 12px 30px rgba(0, 0, 0, 0.35);
  }
  .mobile-nav button { text-align: left; padding: 12px 14px; border: 0; border-radius: 8px; color: var(--text-primary); background: transparent; font-size: 15px; }
  .mobile-nav button.active, .mobile-nav button:hover { background: rgba(91, 156, 245, 0.18); }
}
</style>
