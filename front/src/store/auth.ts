import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import axios from '../axios';

const ACCESS_KEY = 'accessToken';
const REFRESH_KEY = 'refreshToken';

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(localStorage.getItem(ACCESS_KEY));
  const refreshToken = ref<string | null>(localStorage.getItem(REFRESH_KEY));
  const currentUser = ref<{ username: string; id: number } | null>(null);

  const isAuthenticated = computed(() => !!accessToken.value);

  window.addEventListener('auth:tokens', (event: Event) => {
    const tokens = (event as CustomEvent<{ access: string; refresh: string } | null>).detail;
    accessToken.value = tokens?.access || null;
    refreshToken.value = tokens?.refresh || null;
    if (!tokens) currentUser.value = null;
  });

  // 兼容旧逻辑：token 即 accessToken
  const token = accessToken;

  const saveTokens = (access: string, refresh: string) => {
    accessToken.value = access;
    refreshToken.value = refresh;
    localStorage.setItem(ACCESS_KEY, access);
    localStorage.setItem(REFRESH_KEY, refresh);
  };

  const clearTokens = () => {
    accessToken.value = null;
    refreshToken.value = null;
    currentUser.value = null;
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
    localStorage.removeItem('token');
  };

  const applyAuthResponse = (data: {
    accessToken?: string;
    refreshToken?: string;
    token?: string;
  }) => {
    const access = data.accessToken || data.token;
    const refresh = data.refreshToken;
    if (!access || !refresh) {
      throw new Error('Invalid auth response');
    }
    saveTokens(access, refresh);
  };

  const login = async (
    username: string,
    password: string,
    captchaId: string,
    captchaValue: string
  ) => {
    try {
      const response = await axios.post('/auth/login', {
        username,
        password,
        captchaId,
        captchaValue,
      });
      applyAuthResponse(response.data);
      await fetchCurrentUser();
      return response.data;
    } catch (error: any) {
      console.error('Login error:', error);
      throw new Error(error.response?.data?.error || 'Login failed');
    }
  };

  const register = async (username: string, password: string) => {
    try {
      const response = await axios.post('/auth/register', { username, password });
      applyAuthResponse(response.data);
      await fetchCurrentUser();
      return response.data;
    } catch (error: any) {
      console.error('Register error:', error);
      throw new Error(error.response?.data?.error || 'Register failed');
    }
  };

  const refreshAccessToken = async (): Promise<boolean> => {
    if (!refreshToken.value) {
      return false;
    }
    try {
      const response = await axios.post('/auth/refresh', {
        refreshToken: refreshToken.value,
      });
      applyAuthResponse(response.data);
      return true;
    } catch (error) {
      console.error('Refresh token failed:', error);
      clearTokens();
      return false;
    }
  };

  const fetchCurrentUser = async () => {
    if (!accessToken.value) {
      currentUser.value = null;
      return;
    }
    try {
      const response = await axios.get('/user/me');
      currentUser.value = response.data;
    } catch (error: any) {
      console.error('Failed to fetch current user:', error);
      clearTokens();
    }
  };

  const logout = async () => {
    try {
      if (accessToken.value) {
        await axios.post(
          '/auth/logout',
          { refreshToken: refreshToken.value || '' },
          { headers: { Authorization: accessToken.value } }
        );
      }
    } catch (error) {
      console.error('Logout API error:', error);
    } finally {
      clearTokens();
    }
  };

  return {
    token,
    accessToken,
    refreshToken,
    currentUser,
    isAuthenticated,
    login,
    register,
    logout,
    fetchCurrentUser,
    refreshAccessToken,
    clearTokens,
  };
});
