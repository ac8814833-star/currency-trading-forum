<template>
  <el-container>
    <el-main class="page-shell">
      <div class="rag-card surface-card">
        <div class="rag-glow"></div>
        <div class="rag-header">
          <div class="rag-header-left">
            <div class="rag-icon">◉</div>
            <div>
              <h2 class="rag-title gradient-text">RAG 智能问答助手</h2>
              <p class="rag-subtitle">基于社区文章的 AI 知识检索 —— 试试问我外汇交易相关的问题</p>
            </div>
          </div>
        </div>

        <div class="rag-input-row">
          <el-input
            v-model="question"
            placeholder="例如：美元兑人民币最近的走势怎么样？"
            size="large"
            class="question-input"
            @keyup.enter="askQuestion"
          />
          <el-button type="primary" size="large" round @click="askQuestion" :loading="loading">
            查询
          </el-button>
        </div>

        <div class="rag-result" v-if="loading || result || error" aria-live="polite">
          <div v-if="loading" class="result-loading">
            <div class="loading-dots">
              <span></span><span></span><span></span>
            </div>
            <p>正在分析中，请稍候...</p>
          </div>

          <div v-if="error && !loading" class="result-error">
            <el-alert :title="error" type="error" :closable="false" show-icon />
          </div>

          <div v-if="result && !loading" class="result-success">
            <div class="result-label">✦ AI 回答</div>
            <div class="result-content">
              <template v-for="(part, index) in answerParts" :key="index">
                <router-link
                  v-if="part.articleId"
                  :to="{ name: 'NewsDetail', params: { id: part.articleId } }"
                  class="inline-citation"
                >{{ part.text }}</router-link>
                <span v-else>{{ part.text }}</span>
              </template>
            </div>
            <div v-if="hits.length" class="citation-hint">回答中的 [文章 N] 可点击查看对应原文。</div>

            <div v-if="hits && hits.length" class="result-refs">
              <div class="refs-title">相关文章参考</div>
              <router-link v-for="(hit, index) in hits" :key="index" :to="{ name: 'NewsDetail', params: { id: hit.article_id } }" class="ref-item">
                <div class="ref-title">{{ hit.title || `文章 #${hit.article_id}` }}</div>
                <div class="ref-preview">查看原文 · {{ retrievalSourceLabel(hit.source) }} · 更新于 {{ formatArticleDate(hit.article_updated_at) }}</div>
                <div v-if="hit.excerpt" class="ref-excerpt">{{ hit.excerpt }}</div>
                <div v-if="hit.rerank_score !== undefined" class="ref-score">重排相关度 {{ hit.rerank_score.toFixed(3) }}</div>
              </router-link>
            </div>
          </div>
        </div>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage } from "element-plus";
import axios from "../axios";
import { useAuthStore } from "../store/auth";

interface RAGHit {
  article_id: number;
  title: string;
  source?: string;
  article_updated_at?: string;
  excerpt?: string;
  rank?: number;
  rerank_score?: number;
}

interface AnswerPart {
  text: string;
  articleId?: number;
}

const authStore = useAuthStore();
const question = ref("");
const result = ref("");
const hits = ref<RAGHit[]>([]);
const error = ref("");
const loading = ref(false);

const answerParts = computed<AnswerPart[]>(() => {
  const sourceIDs = new Set(hits.value.map((hit) => hit.article_id));
  const parts: AnswerPart[] = [];
  const citationPattern = /\[文章\s*(\d+)\]/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = citationPattern.exec(result.value)) !== null) {
    if (match.index > lastIndex) {
      parts.push({ text: result.value.slice(lastIndex, match.index) });
    }
    const articleId = Number(match[1]);
    parts.push({
      text: match[0],
      articleId: sourceIDs.has(articleId) ? articleId : undefined,
    });
    lastIndex = citationPattern.lastIndex;
  }
  if (lastIndex < result.value.length || parts.length === 0) {
    parts.push({ text: result.value.slice(lastIndex) });
  }
  return parts;
});

const retrievalSourceLabel = (source?: string) => {
  if (source?.includes("hybrid")) return "混合检索";
  if (source === "keyword") return "关键词降级检索";
  return "向量检索";
};

const formatArticleDate = (value?: string) => {
  if (!value) return "日期未知";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "日期未知";
  return date.toLocaleDateString("zh-CN");
};

const askQuestion = async () => {
  if (!authStore.isAuthenticated) {
    ElMessage.error("请先登录后再使用 RAG 助手");
    return;
  }
  if (!question.value.trim()) {
    ElMessage.warning("请输入问题");
    return;
  }
  loading.value = true;
  error.value = "";
  result.value = "";
  hits.value = [];
  try {
    const response = await axios.post("/rag/ask", {
      question: question.value.trim(),
      top_k: 5,
    });
    const data = response.data;
    result.value = data.answer || "";
    hits.value = data.hits || [];
  } catch (err: any) {
    error.value = err.response?.data?.error || "查询失败，请重试";
  } finally {
    loading.value = false;
  }
};
</script>

<style scoped>
.rag-card {
  position: relative;
  max-width: 800px;
  margin: 0 auto;
  padding: 36px;
  overflow: hidden;
}

.rag-glow {
  position: absolute;
  top: -100px;
  right: -100px;
  width: 300px;
  height: 300px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.1), transparent 70%);
  pointer-events: none;
}

.rag-header {
  margin-bottom: 28px;
}

.rag-header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.rag-icon {
  font-size: 40px;
  color: var(--accent-cyan);
  animation: iconFloat 3s ease-in-out infinite;
}

@keyframes iconFloat {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}

.rag-title {
  margin: 0;
  font-size: 28px;
}

.rag-subtitle {
  margin: 6px 0 0;
  color: var(--text-muted);
  font-size: 14px;
}

.rag-input-row {
  display: flex;
  gap: 12px;
  margin-bottom: 32px;
}

.question-input {
  flex: 1;
}

/* ── Loading ── */
.result-loading {
  text-align: center;
  padding: 40px 0;
  color: var(--text-muted);
}

.loading-dots {
  display: flex;
  gap: 8px;
  justify-content: center;
  margin-bottom: 16px;
}

.loading-dots span {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent-blue);
  animation: dotBounce 1.2s ease-in-out infinite;
}

.loading-dots span:nth-child(1) { animation-delay: 0s; }
.loading-dots span:nth-child(2) { animation-delay: 0.2s; }
.loading-dots span:nth-child(3) { animation-delay: 0.4s; }

@keyframes dotBounce {
  0%, 80%, 100% { transform: scale(0.4); opacity: 0.3; }
  40% { transform: scale(1); opacity: 1; }
}

/* ── Result ── */
.result-label {
  font-weight: 700;
  font-size: 16px;
  color: var(--accent-cyan);
  margin-bottom: 12px;
}

.result-content {
  padding: 20px;
  background: rgba(91, 156, 245, 0.06);
  border: 1px solid rgba(100, 150, 240, 0.12);
  border-radius: 14px;
  line-height: 1.8;
  white-space: pre-wrap;
  color: var(--text-primary);
}

.inline-citation {
  color: var(--accent-cyan);
  font-weight: 700;
  text-decoration: none;
}

.inline-citation:hover {
  text-decoration: underline;
}

.citation-hint {
  margin-top: 8px;
  color: var(--text-muted);
  font-size: 12px;
}

.result-refs {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid rgba(100, 150, 240, 0.1);
}

.refs-title {
  font-weight: 600;
  margin-bottom: 12px;
  color: var(--text-secondary);
  font-size: 14px;
}

.ref-item {
  display: block;
  text-decoration: none;
  padding: 12px;
  margin-bottom: 8px;
  background: rgba(100, 160, 255, 0.04);
  border: 1px solid rgba(100, 150, 240, 0.1);
  border-radius: 10px;
  transition: all 0.3s ease;
}

.ref-item:hover {
  border-color: rgba(100, 160, 255, 0.2);
  background: rgba(100, 160, 255, 0.07);
}

.ref-title {
  font-weight: 600;
  color: var(--accent-blue);
  font-size: 14px;
  margin-bottom: 4px;
}

.ref-preview {
  font-size: 13px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ref-excerpt {
  display: -webkit-box;
  margin-top: 6px;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.ref-score {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 4px;
}

.result-error {
  margin-top: 16px;
}

@media (max-width: 600px) {
  .rag-card { padding: 22px 16px; }
  .rag-title { font-size: 23px; }
  .rag-input-row { flex-direction: column; }
  .rag-input-row .el-button { width: 100%; }
}
</style>
