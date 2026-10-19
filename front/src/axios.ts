import axios, { type AxiosError, type InternalAxiosRequestConfig } from 'axios';

const instance = axios.create({
  baseURL: import.meta.env.DEV ? 'http://localhost:3000/api' : '/api',
});

let isRefreshing = false;
let refreshQueue: Array<(token: string | null) => void> = [];

const getAccessToken = () =>
  localStorage.getItem('accessToken') || localStorage.getItem('token');

const getRefreshToken = () => localStorage.getItem('refreshToken');

const saveTokens = (access: string, refresh: string) => {
  localStorage.setItem('accessToken', access);
  localStorage.setItem('refreshToken', refresh);
  localStorage.setItem('token', access);
  window.dispatchEvent(new CustomEvent('auth:tokens', { detail: { access, refresh } }));
};

const clearTokens = () => {
  localStorage.removeItem('accessToken');
  localStorage.removeItem('refreshToken');
  localStorage.removeItem('token');
  window.dispatchEvent(new CustomEvent('auth:tokens', { detail: null }));
};

instance.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) {
    config.headers.Authorization = token;
  }
  return config;
});

instance.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean };

    if (error.response?.status !== 401 || !originalRequest || originalRequest._retry) {
      return Promise.reject(error);
    }

    const url = originalRequest.url || '';
    if (url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/refresh')) {
      return Promise.reject(error);
    }

    const refresh = getRefreshToken();
    if (!refresh) {
      clearTokens();
      return Promise.reject(error);
    }

    if (isRefreshing) {
      return new Promise((resolve, reject) => {
        refreshQueue.push((newAccess) => {
          if (!newAccess) {
            reject(error);
            return;
          }
          originalRequest.headers.Authorization = newAccess;
          resolve(instance(originalRequest));
        });
      });
    }

    originalRequest._retry = true;
    isRefreshing = true;

    try {
      const res = await axios.post(
        `${import.meta.env.DEV ? 'http://localhost:3000/api' : '/api'}/auth/refresh`,
        { refreshToken: refresh }
      );
      const access = res.data.accessToken || res.data.token;
      const newRefresh = res.data.refreshToken;
      if (!access || !newRefresh) {
        throw new Error('refresh failed');
      }
      saveTokens(access, newRefresh);
      refreshQueue.forEach((cb) => cb(access));
      refreshQueue = [];
      originalRequest.headers.Authorization = access;
      return instance(originalRequest);
    } catch (refreshErr) {
      refreshQueue.forEach((cb) => cb(null));
      refreshQueue = [];
      clearTokens();
      return Promise.reject(refreshErr);
    } finally {
      isRefreshing = false;
    }
  }
);

export default instance;
