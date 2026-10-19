<template>
  <div class="auth-container">
    <div class="auth-card surface-card">
      <div class="auth-glow"></div>
      <div class="auth-icon">◇</div>
      <h2 class="auth-title">欢迎回来</h2>
      <p class="auth-subtitle">登录后可发布文章并参与互动</p>
      <el-form :model="form" class="auth-form" @submit.prevent="login">
        <el-form-item>
          <el-input v-model="form.username" placeholder="请输入用户名" prefix-icon="User" size="large" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="form.password" type="password" placeholder="请输入密码" prefix-icon="Lock" size="large" show-password />
        </el-form-item>
        <el-form-item>
          <div class="captcha-row">
            <el-input v-model="form.captchaValue" placeholder="验证码" size="large" class="captcha-input" />
            <img v-if="captchaImage" :src="captchaImage" alt="验证码" class="captcha-img" @click="fetchCaptcha" />
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit" size="large" round class="auth-btn">登 录</el-button>
        </el-form-item>
        <div class="switch-tip">
          还没有账号？
          <el-button link type="primary" @click="router.push({ name: 'Register' })">去注册 →</el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../store/auth';
import { ElMessage } from 'element-plus';
import axios from '../axios';

const form = ref({
  username: '',
  password: '',
  captchaId: '',
  captchaValue: '',
});

const captchaImage = ref('');
const authStore = useAuthStore();
const router = useRouter();

const fetchCaptcha = async () => {
  try {
    const response = await axios.get('/auth/captcha');
    form.value.captchaId = response.data.captchaId;
    captchaImage.value = response.data.captchaImage;
  } catch (error) {
    ElMessage.error('获取验证码失败');
  }
};

onMounted(fetchCaptcha);

const login = async () => {
  if (!form.value.username.trim()) { ElMessage.warning('请输入用户名'); return; }
  if (!form.value.password.trim()) { ElMessage.warning('请输入密码'); return; }
  if (!form.value.captchaValue.trim()) { ElMessage.warning('请输入验证码'); return; }
  try {
    await authStore.login(form.value.username, form.value.password, form.value.captchaId, form.value.captchaValue);
    ElMessage.success('登录成功');
    router.push({ name: 'News' });
  } catch (error: any) {
    ElMessage.error(error.message || '登录失败，请检查用户名、密码或验证码。');
    fetchCaptcha();
    form.value.captchaValue = '';
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
  background: radial-gradient(circle, rgba(91, 156, 245, 0.15), transparent 70%);
  pointer-events: none;
  animation: authGlow 4s ease-in-out infinite alternate;
}

@keyframes authGlow {
  0% { opacity: 0.3; transform: translateX(-50%) scale(1); }
  100% { opacity: 0.7; transform: translateX(-50%) scale(1.3); }
}

.auth-icon {
  font-size: 32px;
  color: var(--accent-cyan);
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

.captcha-row {
  display: flex;
  gap: 10px;
  align-items: center;
}

.captcha-input { flex: 1; }

.captcha-img {
  height: 40px;
  cursor: pointer;
  border-radius: 8px;
  border: 1px solid rgba(100, 150, 240, 0.2);
  transition: all 0.3s ease;
}

.captcha-img:hover {
  border-color: var(--accent-cyan);
  box-shadow: 0 0 12px rgba(34, 211, 238, 0.2);
}

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

/* Override Element Plus input prefix icon color */
:deep(.el-input__prefix-inner) {
  color: var(--text-muted) !important;
}
</style>
