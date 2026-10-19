<template>
  <div class="auth-container">
    <div class="auth-card surface-card">
      <div class="auth-glow"></div>
      <div class="auth-icon">◆</div>
      <h2 class="auth-title">创建新账号</h2>
      <p class="auth-subtitle">加入社区，分享你的交易洞察</p>
      <el-form :model="form" class="auth-form" @submit.prevent="register">
        <el-form-item>
          <el-input v-model="form.username" placeholder="请输入用户名" prefix-icon="User" size="large" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="form.password" type="password" placeholder="请输入密码（至少6位）" prefix-icon="Lock" size="large" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit" size="large" round class="auth-btn">注 册</el-button>
        </el-form-item>
        <div class="switch-tip">
          已有账号？
          <el-button link type="primary" @click="router.push({ name: 'Login' })">去登录 →</el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../store/auth';
import { ElMessage } from 'element-plus';

const form = ref({
  username: '',
  password: '',
});

const authStore = useAuthStore();
const router = useRouter();

const register = async () => {
  if (!form.value.username.trim()) { ElMessage.warning('请输入用户名'); return; }
  if (!form.value.password.trim()) { ElMessage.warning('请输入密码'); return; }
  if (form.value.password.length < 6) { ElMessage.warning('密码长度至少为6位'); return; }
  try {
    await authStore.register(form.value.username, form.value.password);
    ElMessage.success('注册成功');
    router.push({ name: 'News' });
  } catch (error: any) {
    ElMessage.error(error.message || '注册失败，请重试。');
  }
};
</script>

<style scoped>
.auth-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: calc(100vh - 76px);
  padding: 20px;
}

.auth-card {
  position: relative;
  width: 100%;
  max-width: 430px;
  padding: 40px 32px 28px;
  text-align: center;
  overflow: hidden;
}

.auth-glow {
  position: absolute;
  top: -60px;
  left: 50%;
  transform: translateX(-50%);
  width: 200px;
  height: 200px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(167, 139, 250, 0.15), transparent 70%);
  pointer-events: none;
  animation: authGlow 4s ease-in-out infinite alternate;
}

@keyframes authGlow {
  0% { opacity: 0.3; transform: translateX(-50%) scale(1); }
  100% { opacity: 0.7; transform: translateX(-50%) scale(1.3); }
}

.auth-icon {
  font-size: 32px;
  color: var(--accent-purple);
  margin-bottom: 12px;
  animation: iconFloat 3s ease-in-out infinite;
  display: inline-block;
}

@keyframes iconFloat {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}

.auth-title {
  margin: 0;
  font-size: 26px;
  font-weight: 700;
  color: var(--text-primary);
}

.auth-subtitle {
  margin: 8px 0 24px;
  color: var(--text-muted);
  font-size: 14px;
}

.auth-form { text-align: left; }

.auth-btn {
  width: 100%;
  margin-top: 8px;
}

.switch-tip {
  margin-top: 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

:deep(.el-input__prefix-inner) {
  color: var(--text-muted) !important;
}
</style>
