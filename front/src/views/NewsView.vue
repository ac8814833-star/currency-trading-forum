<template>
  <el-container class="news-page">
    <el-main class="page-shell">
      <div class="page-header">
        <div>
          <h2 class="section-title gradient-text">外汇交易 · 新闻与讨论</h2>
          <p class="section-subtitle">查看最新观点、热点话题与社区讨论</p>
        </div>
        <div v-if="authStore.isAuthenticated" class="header-actions">
          <el-input
            v-model="keyword"
            clearable
            placeholder="搜索文章标题、摘要或内容"
            class="search-input"
            @keyup.enter="handleSearch"
            @clear="clearSearch"
          >
            <template #append>
              <el-button :loading="searching" @click="handleSearch">搜索</el-button>
            </template>
          </el-input>
          <el-button
            v-if="authStore.isAuthenticated"
            type="primary"
            round
            @click="router.push({ name: 'CreateArticle' })"
            :icon="Plus"
          >
            发布新文章
          </el-button>
        </div>
        <el-button v-else type="primary" round @click="router.push({ name: 'Login' })">登录后浏览社区文章</el-button>
      </div>

      <div v-if="isSearching" class="search-tip">
        搜索「{{ activeKeyword }}」共 {{ searchTotal }} 条结果
        <span v-if="searchSource" class="search-source">（{{ searchSourceLabel }}）</span>
        <el-button link type="primary" @click="clearSearch">清除搜索</el-button>
      </div>

      <el-row :gutter="20">
        <el-col :md="16" :sm="24">
          <div v-if="listLoading" class="no-data" role="status">正在加载文章...</div>
          <div v-else-if="listError" class="no-data">
            <p>{{ listError }}</p><el-button @click="reloadList">重试</el-button>
          </div>
          <div v-else-if="displayArticles && displayArticles.length" class="list-section">
            <div v-for="article in displayArticles" :key="article.ID" class="article-card surface-card" @click="viewDetail(article.ID)">
              <div class="article-header">
                <h3 class="article-title">{{ article.Title }}</h3>
                <span class="likes-badge" v-if="article.LikesCount">♥ {{ article.LikesCount }}</span>
              </div>
              <p class="article-preview">{{ article.Preview }}</p>
              <div class="article-footer">
                <div class="author-info">
                  <span class="article-author">@{{ article.Username }}</span>
                  <span class="article-date">{{ article.CreatedAt ? new Date(article.CreatedAt).toLocaleDateString('zh-CN') : '' }}</span>
                </div>
                <el-button text type="primary" class="read-more">阅读全文 →</el-button>
              </div>
            </div>
            <el-pagination class="article-pagination" :current-page="displayPage" :page-size="20" :total="displayTotal" layout="prev, pager, next" @current-change="changePage" />
          </div>
          <div v-else-if="!authStore.isAuthenticated" class="no-data">
            <el-empty description="请先登录后再查看文章" />
          </div>
          <div v-else-if="isSearching" class="no-data">
            <el-empty description="没有找到相关文章" />
          </div>
          <div v-else class="no-data">
            <el-empty description="暂无文章，快来发布第一篇吧！" />
          </div>
        </el-col>

        <el-col v-if="authStore.isAuthenticated" :md="8" :sm="24">
          <div class="ranking-card surface-card">
            <div class="ranking-header">
              <span class="ranking-icon">◆</span>
              <span class="ranking-title">热门文章排行榜</span>
            </div>
            <div v-if="hotArticles && hotArticles.length" class="ranking-list">
              <div
                v-for="(article, index) in hotArticles"
                :key="article.ID"
                class="ranking-item"
                @click="viewDetail(article.ID)"
              >
                <div class="ranking-index" :class="'rank-' + (index + 1)">
                  <template v-if="index < 3">
                    {{ ['🥇', '🥈', '🥉'][index] }}
                  </template>
                  <template v-else>{{ index + 1 }}</template>
                </div>
                <div class="ranking-info">
                  <div class="ranking-item-title">{{ article.Title }}</div>
                  <div class="ranking-item-meta">
                    <span>@{{ article.Username }}</span>
                    <span class="ranking-likes">♥ {{ article.likes }}</span>
                  </div>
                </div>
              </div>
            </div>
            <el-empty v-else :image-size="60" description="暂无排行" />
          </div>
        </el-col>
      </el-row>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { Plus } from '@element-plus/icons-vue';
import axios from '../axios';
import { useAuthStore } from '../store/auth';
import type { Article } from "../types/Article";

const articles = ref<Article[]>([]);
const searchResults = ref<Article[]>([]);
const hotArticles = ref<Array<Article & { likes: number }>>([]);
const keyword = ref('');
const activeKeyword = ref('');
const isSearching = ref(false);
const searching = ref(false);
const searchTotal = ref(0);
const searchSource = ref('');
const articlePage = ref(1);
const articleTotal = ref(0);
const searchPage = ref(1);
const listLoading = ref(false);
const listError = ref('');
const router = useRouter();
const authStore = useAuthStore();

const displayArticles = computed(() => (isSearching.value ? searchResults.value : articles.value));
const displayPage = computed(() => (isSearching.value ? searchPage.value : articlePage.value));
const displayTotal = computed(() => (isSearching.value ? searchTotal.value : articleTotal.value));
const searchSourceLabel = computed(() =>
  searchSource.value === 'elasticsearch' ? 'Elasticsearch' : searchSource.value === 'mysql' ? 'MySQL 降级' : ''
);

const fetchArticles = async () => {
  if (!authStore.isAuthenticated) return;
  listLoading.value = true;
  listError.value = '';
  try {
    const response = await axios.get<{ data: Article[]; total: number }>('/articles', { params: { page: articlePage.value, pageSize: 20 } });
    articles.value = response.data.data || [];
    articleTotal.value = response.data.total || 0;
  } catch (error) {
    listError.value = '文章加载失败，请重试';
  } finally {
    listLoading.value = false;
  }
};

const fetchHotArticles = async () => {
  if (!authStore.isAuthenticated) return;
  try {
    const response = await axios.get<{ data: Array<Article & { likes: number }> }>('/articles/hot', {
      params: { limit: 10 },
    });
    hotArticles.value = response.data.data;
  } catch (error) {
    console.error('Failed to load hot articles:', error);
  }
};

const handleSearch = async () => {
  if (!authStore.isAuthenticated) {
    ElMessage.warning('请先登录后再搜索');
    return;
  }
  const q = keyword.value.trim();
  if (!q) {
    clearSearch();
    return;
  }
  searchPage.value = 1;
  await fetchSearch(q);
};

const fetchSearch = async (q: string) => {
  searching.value = true;
  listLoading.value = true;
  listError.value = '';
  activeKeyword.value = q;
  isSearching.value = true;
  try {
    const response = await axios.get<{
      articles: Article[];
      total: number;
      source: string;
    }>('/articles/search', {
      params: { q, page: searchPage.value, pageSize: 20 },
    });
    searchResults.value = response.data.articles || [];
    searchTotal.value = response.data.total || 0;
    searchSource.value = response.data.source || '';
    activeKeyword.value = q;
    isSearching.value = true;
  } catch (error: any) {
    listError.value = error.response?.data?.error || '搜索失败';
  } finally {
    searching.value = false;
    listLoading.value = false;
  }
};

const changePage = (page: number) => {
  if (isSearching.value) {
    searchPage.value = page;
    void fetchSearch(activeKeyword.value);
  } else {
    articlePage.value = page;
    void fetchArticles();
  }
};

const reloadList = () => isSearching.value ? fetchSearch(activeKeyword.value) : fetchArticles();

const clearSearch = () => {
  keyword.value = '';
  activeKeyword.value = '';
  isSearching.value = false;
  searchResults.value = [];
  searchTotal.value = 0;
  searchSource.value = '';
  searchPage.value = 1;
  listError.value = '';
};

const viewDetail = (id: string) => {
  if (!authStore.isAuthenticated) {
    ElMessage.error('请先登录后再查看');
    return;
  }
  router.push({ name: 'NewsDetail', params: { id } });
};

onMounted(() => { void Promise.all([fetchArticles(), fetchHotArticles()]); });
</script>

<style scoped>
.news-page {
  min-height: calc(100vh - 76px);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  gap: 12px;
  flex-wrap: wrap;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  width: min(360px, 100%);
}

.search-tip {
  margin: -8px 0 16px;
  color: var(--text-muted);
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.search-source {
  color: #64748b;
}

.section-title {
  margin: 0;
  font-size: 30px;
}

.section-subtitle {
  margin: 8px 0 0;
  color: var(--text-muted);
  font-size: 14px;
}

/* ── 文章卡片 ── */
.article-card {
  margin-bottom: 14px;
  padding: 24px;
  cursor: pointer;
  transition: all 0.35s ease;
  position: relative;
  overflow: hidden;
}

.article-card::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: radial-gradient(circle at 50% 0%, rgba(91, 156, 245, 0.05), transparent 60%);
  opacity: 0;
  transition: opacity 0.35s ease;
}

.article-card:hover::after {
  opacity: 1;
}

.article-card:hover {
  border-color: rgba(100, 160, 255, 0.25);
  transform: translateY(-2px);
}

.article-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
}

.article-title {
  margin: 0;
  font-size: 19px;
  color: var(--text-primary);
  font-weight: 600;
  line-height: 1.4;
}

.likes-badge {
  font-size: 13px;
  color: #f87171;
  background: rgba(248, 113, 113, 0.1);
  border: 1px solid rgba(248, 113, 113, 0.2);
  padding: 3px 10px;
  border-radius: 20px;
  white-space: nowrap;
  margin-left: 12px;
}

.article-preview {
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.65;
  margin-bottom: 16px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.article-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.author-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.article-author {
  font-size: 13px;
  color: var(--text-muted);
}

.article-date {
  font-size: 12px;
  color: var(--text-muted);
  opacity: 0.7;
}

.read-more {
  color: var(--accent-blue) !important;
  transition: all 0.3s ease;
}

.article-card:hover .read-more {
  color: var(--accent-cyan) !important;
}

/* ── 排行榜 ── */
.ranking-card {
  padding: 20px;
  position: sticky;
  top: 92px;
}

.ranking-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid rgba(100, 150, 240, 0.1);
}

.ranking-icon {
  color: var(--accent-cyan);
  font-size: 14px;
}

.ranking-title {
  font-weight: 700;
  font-size: 16px;
  color: var(--text-primary);
}

.ranking-item {
  display: flex;
  align-items: center;
  padding: 12px;
  cursor: pointer;
  transition: all 0.3s ease;
  border-radius: 10px;
  margin-bottom: 4px;
}

.ranking-item:hover {
  background: rgba(100, 160, 255, 0.06);
  border: 1px solid rgba(100, 160, 255, 0.12);
}

.ranking-index {
  width: 30px;
  height: 30px;
  line-height: 30px;
  text-align: center;
  border-radius: 50%;
  margin-right: 12px;
  font-weight: 700;
  font-size: 13px;
  background: rgba(100, 150, 240, 0.08);
  color: var(--text-muted);
  flex-shrink: 0;
}

.rank-1 { background: linear-gradient(135deg, #ffd700, #ffb800); color: #1a1a2e; box-shadow: 0 0 12px rgba(255, 215, 0, 0.3); }
.rank-2 { background: linear-gradient(135deg, #c0c0c0, #a8a8a8); color: #1a1a2e; box-shadow: 0 0 10px rgba(192, 192, 192, 0.3); }
.rank-3 { background: linear-gradient(135deg, #cd7f32, #b8860b); color: #1a1a2e; box-shadow: 0 0 10px rgba(205, 127, 50, 0.3); }

.ranking-info {
  flex: 1;
  min-width: 0;
}

.ranking-item-title {
  font-size: 14px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 4px;
}

.ranking-item-meta {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--text-muted);
}

.ranking-likes {
  color: #f87171;
}

.no-data {
  padding: 64px 0;
  text-align: center;
}

.article-pagination { justify-content: center; margin: 22px 0; }

@media (max-width: 768px) {
  .section-title { font-size: 24px; }
  .page-header { flex-direction: column; align-items: flex-start; }
  .header-actions, .search-input { width: 100%; }
  .article-card { padding: 18px 16px; }
  .article-header { gap: 8px; }
  .article-title { overflow-wrap: anywhere; }
}
</style>
