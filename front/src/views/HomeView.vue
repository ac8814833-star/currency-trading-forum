<template>
  <div class="page-shell home-page">
    <section class="hero surface-card">
      <div class="hero-glow"></div>
      <div class="hero-content">
        <div class="hero-badge">
          <span class="badge-dot"></span>
          全球外币交易社区
        </div>
        <h1 class="hero-title">
          欢迎来到
          <span class="gradient-text">星汇论坛</span>
        </h1>
        <p class="hero-desc">
          使用系统汇率参考数据进行换算，阅读社区文章，与交易爱好者交流观点
        </p>
        <div class="action-buttons">
          <el-button type="primary" size="large" round @click="router.push({ name: 'News' })">
            <span class="btn-icon">◈</span> 浏览社区讨论
          </el-button>
          <el-button type="success" size="large" round plain @click="router.push({ name: 'CurrencyExchange' })">
            <span class="btn-icon">◎</span> 汇率兑换工具
          </el-button>
        </div>
        <div v-if="!authStore.isAuthenticated" class="login-tip">
          <span class="tip-glow"></span>
          登录后可发布文章、点赞与评论
          <el-button link type="primary" @click="router.push({ name: 'Login' })">立即登录 →</el-button>
        </div>
      </div>
      <div class="hero-visual">
        <div class="floating-card" v-for="n in 4" :key="n" :class="'card-' + n">
          <div class="fc-dot"></div>
          <div class="fc-line"></div>
          <div class="fc-line short"></div>
        </div>
        <div class="hero-ring">
          <div class="ring r1"></div>
          <div class="ring r2"></div>
          <div class="ring r3"></div>
        </div>
      </div>
    </section>

    <div class="features-grid">
      <div class="feature-card surface-card" v-for="feat in features" :key="feat.label">
        <div class="feat-icon">{{ feat.icon }}</div>
        <div class="feat-label">{{ feat.label }}</div>
        <div class="feat-value">{{ feat.value }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useAuthStore } from '../store/auth';

const router = useRouter();
const authStore = useAuthStore();

const features = [
  { icon: '◈', label: '内容模块', value: '资讯 + 交流' },
  { icon: '◎', label: '交易工具', value: '参考汇率换算' },
  { icon: '✦', label: '互动能力', value: '点赞 + 评论' },
  { icon: '◉', label: 'AI 助手', value: 'RAG 问答' },
];
</script>

<style scoped>
.home-page {
  padding-top: 12px;
}

.hero {
  position: relative;
  padding: 50px 44px;
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 36px;
  overflow: hidden;
  border: 1px solid rgba(100, 160, 255, 0.14);
}

.hero-glow {
  position: absolute;
  top: -80px;
  right: -60px;
  width: 350px;
  height: 350px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(91, 156, 245, 0.15) 0%, transparent 70%);
  pointer-events: none;
  animation: heroGlow 6s ease-in-out infinite alternate;
}

@keyframes heroGlow {
  0% { transform: translate(0, 0) scale(1); opacity: 0.5; }
  100% { transform: translate(-30px, 20px) scale(1.2); opacity: 0.8; }
}

.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 18px;
  border-radius: 999px;
  background: rgba(91, 156, 245, 0.1);
  border: 1px solid rgba(91, 156, 245, 0.2);
  color: var(--accent-cyan);
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 20px;
  backdrop-filter: blur(10px);
}

.badge-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent-cyan);
  box-shadow: 0 0 8px var(--accent-cyan);
  animation: dotPulse 2s ease-in-out infinite;
}

@keyframes dotPulse {
  0%, 100% { opacity: 0.5; }
  50% { opacity: 1; }
}

.hero-title {
  margin: 0 0 18px;
  font-size: 46px;
  line-height: 1.25;
  color: var(--text-primary);
  font-weight: 800;
  letter-spacing: -0.5px;
}

.hero-desc {
  margin: 0;
  color: var(--text-secondary);
  font-size: 17px;
  line-height: 1.8;
  max-width: 480px;
}

.action-buttons {
  margin-top: 32px;
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
}

.btn-icon {
  font-size: 12px;
  margin-right: 4px;
}

.login-tip {
  margin-top: 24px;
  padding: 14px 18px;
  border-radius: 12px;
  background: rgba(91, 156, 245, 0.06);
  border: 1px solid rgba(91, 156, 245, 0.12);
  color: var(--text-secondary);
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* ── 右侧视觉效果 ── */
.hero-visual {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 260px;
}

.hero-ring {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ring {
  position: absolute;
  border-radius: 50%;
  border: 1.5px solid rgba(100, 160, 255, 0.15);
}

.r1 {
  width: 200px;
  height: 200px;
  animation: ringSpin 20s linear infinite;
}

.r2 {
  width: 260px;
  height: 260px;
  border-color: rgba(167, 139, 250, 0.12);
  animation: ringSpin 25s linear infinite reverse;
}

.r3 {
  width: 320px;
  height: 320px;
  border-color: rgba(34, 211, 238, 0.1);
  animation: ringSpin 30s linear infinite;
}

@keyframes ringSpin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.floating-card {
  position: absolute;
  width: 100px;
  height: 60px;
  background: rgba(20, 32, 70, 0.7);
  border: 1px solid rgba(100, 160, 255, 0.15);
  border-radius: 12px;
  padding: 12px;
  backdrop-filter: blur(10px);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.fc-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent-cyan);
  box-shadow: 0 0 8px var(--accent-cyan);
}

.fc-line {
  height: 4px;
  border-radius: 2px;
  background: rgba(100, 160, 255, 0.2);
}

.fc-line.short {
  width: 60%;
}

.card-1 { top: 10px; right: 30px; animation: cardFloat 5s ease-in-out infinite; animation-delay: 0s; }
.card-2 { top: 90px; left: 20px; animation: cardFloat 6s ease-in-out infinite; animation-delay: 1s; }
.card-3 { bottom: 40px; right: 50px; animation: cardFloat 5.5s ease-in-out infinite; animation-delay: 2s; }
.card-4 { bottom: 80px; left: 80px; animation: cardFloat 6.5s ease-in-out infinite; animation-delay: 3s; }

@keyframes cardFloat {
  0%, 100% { transform: translateY(0) rotate(0deg); }
  50% { transform: translateY(-14px) rotate(2deg); }
}

/* ── 特性卡片网格 ── */
.features-grid {
  margin-top: 28px;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}

.feature-card {
  padding: 28px 20px;
  text-align: center;
  transition: all 0.4s ease;
  cursor: default;
  position: relative;
  overflow: hidden;
}

.feature-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 0;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--accent-cyan), transparent);
  transition: width 0.4s ease;
}

.feature-card:hover::before {
  width: 80%;
}

.feature-card:hover {
  border-color: rgba(100, 160, 255, 0.3);
  transform: translateY(-4px);
}

.feat-icon {
  font-size: 32px;
  margin-bottom: 14px;
  animation: iconFloat 4s ease-in-out infinite;
}

.feature-card:nth-child(1) .feat-icon { animation-delay: 0s; }
.feature-card:nth-child(2) .feat-icon { animation-delay: 0.5s; }
.feature-card:nth-child(3) .feat-icon { animation-delay: 1s; }
.feature-card:nth-child(4) .feat-icon { animation-delay: 1.5s; }

@keyframes iconFloat {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}

.feat-label {
  font-size: 13px;
  color: var(--text-muted);
  margin-bottom: 8px;
}

.feat-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
  background: linear-gradient(135deg, var(--accent-cyan), var(--accent-blue));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

@media (max-width: 900px) {
  .hero {
    grid-template-columns: 1fr;
    padding: 32px 24px;
  }
  .hero-title { font-size: 34px; }
  .hero-visual { min-height: 180px; }
  .features-grid { grid-template-columns: repeat(2, 1fr); }
}

@media (max-width: 500px) {
  .features-grid { grid-template-columns: 1fr; }
}
</style>
