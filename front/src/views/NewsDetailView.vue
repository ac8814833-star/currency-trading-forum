<template>
  <el-container>
    <el-main class="page-shell news-detail-page">
      <el-card v-if="article" class="article-detail surface-card" shadow="never">
        <div class="article-header">
          <h1>{{ article.Title }}</h1>
          <div v-if="isAuthor" class="article-actions-bar">
            <el-button type="primary" size="small" round @click="editArticle">编辑</el-button>
            <el-button type="danger" size="small" round plain @click="deleteArticle">删除</el-button>
          </div>
        </div>
        <div class="article-meta">
          <span class="meta-author">@{{ article.Username }}</span>
          <span class="meta-divider">·</span>
          <span class="meta-date">{{ article.CreatedAt ? new Date(article.CreatedAt).toLocaleDateString('zh-CN') : '' }}</span>
        </div>
        <div class="article-body">{{ article.Content }}</div>
        <div class="article-actions">
          <el-button
            type="primary"
            round
            @click="likeArticle"
            :loading="liking"
            :disabled="liking"
            :class="{ 'is-liked': isLiked }"
          >
            <span class="action-icon">{{ isLiked ? '♥' : '♡' }}</span>
            {{ isLiked ? '已点赞' : '点赞' }} ({{ likes }})
          </el-button>
          <span class="comments-count">评论 · {{ commentCount }}</span>
        </div>
      </el-card>
      <div v-else-if="articleLoading" class="no-data" role="status">正在加载文章...</div>
      <div v-else class="no-data">{{ articleError || '文章不存在' }} <el-button @click="loadPage">重试</el-button></div>

      <!-- 评论区域 -->
      <el-card v-if="article" class="comments-section surface-card" shadow="never">
        <template #header>
          <div class="card-header">
            <span class="comment-icon">◈</span>
            评论 · {{ commentCount }}
          </div>
        </template>

        <div class="comment-form" v-if="authStore.isAuthenticated">
          <el-input
            v-model="newComment"
            type="textarea"
            :rows="3"
            placeholder="写下你的评论..."
            maxlength="500"
            show-word-limit
          />
          <el-button type="primary" round @click="submitComment" :loading="submitting" style="margin-top: 12px">
            发表评论
          </el-button>
        </div>
        <div v-else class="login-prompt">
          <el-alert title="请先登录后再发表评论" type="info" :closable="false" show-icon />
        </div>

        <div class="comments-list" v-if="comments.length > 0">
          <div v-for="comment in comments" :key="comment.ID" class="comment-item">
            <div class="comment-content">
              <div class="comment-header">
                <span class="comment-author">@{{ comment.username }}</span>
                <span class="comment-time">{{ formatTime(comment.CreatedAt) }}</span>
                <el-button
                  v-if="authStore.currentUser && comment.username === authStore.currentUser.username"
                  type="danger"
                  size="small"
                  text
                  @click="deleteComment(comment.ID)"
                >
                  删除
                </el-button>
              </div>
              <p class="comment-text">{{ comment.content }}</p>
              <el-button
                v-if="authStore.isAuthenticated"
                text
                size="small"
                @click="startReply(comment)"
                class="reply-btn"
              >
                回复
              </el-button>
            </div>

            <div v-if="replyingTo === comment.ID" class="reply-form">
              <el-input
                v-model="replyContent"
                type="textarea"
                :rows="2"
                placeholder="写下你的回复..."
                maxlength="300"
                show-word-limit
              />
              <div style="margin-top: 10px; display: flex; gap: 8px;">
                <el-button type="primary" size="small" round @click="submitReply(comment.ID)" :loading="submitting">
                  提交
                </el-button>
                <el-button size="small" round @click="cancelReply">取消</el-button>
              </div>
            </div>

            <div v-if="comment.replies && comment.replies.length > 0" class="replies-list">
              <div v-for="reply in comment.replies" :key="reply.ID" class="reply-item">
                <div class="reply-header">
                  <span class="reply-author">@{{ reply.username }}</span>
                  <span class="reply-time">{{ formatTime(reply.CreatedAt) }}</span>
                  <el-button
                    v-if="authStore.currentUser && reply.username === authStore.currentUser.username"
                    type="danger"
                    size="small"
                    text
                    @click="deleteComment(reply.ID)"
                  >
                    删除
                  </el-button>
                </div>
                <p class="reply-text">{{ reply.content }}</p>
              </div>
            </div>
          </div>

          <div class="pagination-container" v-if="totalComments > pageSize">
            <el-pagination
              v-model:current-page="currentPage"
              v-model:page-size="pageSize"
              :total="totalComments"
              layout="prev, pager, next"
              @current-change="fetchComments"
            />
          </div>
        </div>

        <div v-else class="no-comments">
          <p>暂无评论，快来发表第一条吧！</p>
        </div>
      </el-card>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage, ElMessageBox } from "element-plus";
import axios from "../axios";
import { useAuthStore } from "../store/auth";
import type { Article, Like, Comment, CommentCount, CommentsResponse } from "../types/Article";

const article = ref<Article | null>(null);
const articleLoading = ref(true);
const articleError = ref('');
const route = useRoute();
const router = useRouter();
const likes = ref<number>(0);
const isLiked = ref<boolean>(false);
const liking = ref<boolean>(false);
const comments = ref<Comment[]>([]);
const commentCount = ref<number>(0);
const currentPage = ref<number>(1);
const pageSize = ref<number>(10);
const totalComments = ref<number>(0);
const newComment = ref<string>("");
const replyContent = ref<string>("");
const replyingTo = ref<string | null>(null);
const submitting = ref<boolean>(false);
const authStore = useAuthStore();

const { id } = route.params;

const isAuthor = computed(() => {
  if (!authStore.currentUser || !article.value) return false;
  return authStore.currentUser.username === article.value.Username;
});

const fetchArticle = async () => {
  try {
    const response = await axios.get<Article>(`/articles/${id}`);
    article.value = response.data;
  } catch (error: any) {
    articleError.value = error.response?.status === 404 ? '文章不存在或已删除' : '文章加载失败';
  }
};

const editArticle = () => {
  router.push({ name: 'EditArticle', params: { id } });
};

const deleteArticle = async () => {
  if (!isAuthor.value) {
    ElMessage.error("您没有权限删除此文章");
    return;
  }
  try {
    await ElMessageBox.confirm("确定要删除这篇文章吗？删除后无法恢复！", "警告", {
      confirmButtonText: "确定删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await axios.delete(`/articles/${id}`);
    ElMessage.success("文章已删除");
    router.push({ name: 'News' });
  } catch (error: any) {
    if (error.name !== "Cancel") {
      ElMessage.error(error.response?.data?.error || "删除失败");
    }
  }
};

const likeArticle = async () => {
  if (!authStore.isAuthenticated) { ElMessage.warning("请先登录后再点赞"); return; }
  if (liking.value) return;
  liking.value = true;
  try {
    if (isLiked.value) {
      await axios.post(`/articles/${id}/unlike`);
      ElMessage.success("已取消点赞");
    } else {
      await axios.post(`/articles/${id}/like`);
      ElMessage.success("点赞成功");
    }
    await fetchLike();
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || "操作失败");
  } finally {
    liking.value = false;
  }
};

const fetchLike = async () => {
  try {
    const res = await axios.get<Like>(`articles/${id}/like`);
    likes.value = parseInt(res.data.likes as any);
    isLiked.value = res.data.is_liked;
  } catch (error) {
    console.log("Error fetching likes:", error);
  }
};

const fetchComments = async () => {
  try {
    const res = await axios.get(`/articles/${id}/comments`, {
      params: { page: currentPage.value, pageSize: pageSize.value },
    });
    const payload = res.data;
    if (Array.isArray(payload)) {
      comments.value = payload;
    } else if (payload && Array.isArray(payload.data)) {
      comments.value = payload.data;
      if (typeof payload.total === 'number') totalComments.value = payload.total;
    } else {
      comments.value = [];
    }
  } catch (error) {
    console.log("Error fetching comments:", error);
  }
};

const fetchCommentCount = async () => {
  try {
    const response = await axios.get<CommentCount>(`/articles/${id}/comments/count`);
    commentCount.value = response.data.count;
  } catch (error) {
    console.error("Failed to load comment count:", error);
  }
};

const submitComment = async () => {
  if (!authStore.isAuthenticated) { ElMessage.warning("请先登录"); return; }
  if (!newComment.value.trim()) { ElMessage.warning("评论不能为空"); return; }
  submitting.value = true;
  try {
    await axios.post(`/articles/${id}/comments`, {
      article_id: parseInt(id as string),
      content: newComment.value.trim(),
    });
    ElMessage.success("评论成功");
    newComment.value = "";
    await Promise.all([fetchComments(), fetchCommentCount()]);
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || "评论失败");
  } finally { submitting.value = false; }
};

const startReply = (comment: Comment) => {
  if (!authStore.isAuthenticated) { ElMessage.warning("请先登录"); return; }
  replyingTo.value = comment.ID;
  replyContent.value = "";
};

const cancelReply = () => { replyingTo.value = null; replyContent.value = ""; };

const submitReply = async (parentId: string) => {
  if (!replyContent.value.trim()) { ElMessage.warning("回复不能为空"); return; }
  submitting.value = true;
  try {
    await axios.post(`/articles/${id}/comments`, {
      article_id: parseInt(id as string),
      content: replyContent.value.trim(),
      parent_id: parseInt(parentId),
    });
    ElMessage.success("回复成功");
    cancelReply();
    await Promise.all([fetchComments(), fetchCommentCount()]);
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || "回复失败");
  } finally { submitting.value = false; }
};

const deleteComment = async (commentId: string) => {
  try {
    await ElMessageBox.confirm("确定要删除这条评论吗？", "提示", {
      confirmButtonText: "确定", cancelButtonText: "取消", type: "warning",
    });
    await axios.delete(`/comments/${commentId}`);
    ElMessage.success("评论已删除");
    await Promise.all([fetchComments(), fetchCommentCount()]);
  } catch (error: any) {
    if (error !== "cancel") ElMessage.error(error.response?.data?.error || "删除失败");
  }
};

const formatTime = (timeString: string) => {
  const date = new Date(timeString);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);
  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes}分钟前`;
  if (hours < 24) return `${hours}小时前`;
  if (days < 7) return `${days}天前`;
  return date.toLocaleDateString("zh-CN");
};

const loadPage = async () => {
  articleLoading.value = true;
  articleError.value = '';
  await Promise.all([fetchArticle(), authStore.isAuthenticated ? authStore.fetchCurrentUser() : Promise.resolve()]);
  if (article.value) await Promise.all([fetchLike(), fetchComments(), fetchCommentCount()]);
  articleLoading.value = false;
};

onMounted(() => { void loadPage(); });
</script>

<style scoped>
.news-detail-page {
  padding-top: 8px;
}

@media (max-width: 600px) {
  .article-detail { padding: 18px 14px; }
  .article-header { flex-direction: column; }
  .article-header h1 { font-size: 24px; overflow-wrap: anywhere; }
  .article-actions-bar { flex-wrap: wrap; }
  .article-body { overflow-wrap: anywhere; }
}

.article-detail {
  margin-bottom: 18px;
  padding: 36px;
}

.article-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
  gap: 16px;
}

.article-header h1 {
  margin: 0;
  flex: 1;
  font-size: 32px;
  line-height: 1.35;
  color: var(--text-primary);
  font-weight: 700;
}

.article-actions-bar {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.article-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 24px;
  padding-bottom: 20px;
  border-bottom: 1px solid rgba(100, 150, 240, 0.1);
}

.meta-author { color: var(--accent-blue); font-size: 14px; font-weight: 500; }
.meta-divider { color: var(--text-muted); }
.meta-date { color: var(--text-muted); font-size: 13px; }

.article-body {
  white-space: pre-wrap;
  line-height: 1.9;
  color: var(--text-primary);
  font-size: 15px;
}

.article-actions {
  margin-top: 28px;
  display: flex;
  align-items: center;
  gap: 18px;
  padding-top: 18px;
  border-top: 1px solid rgba(100, 150, 240, 0.1);
}

.action-icon {
  font-size: 16px;
  margin-right: 2px;
}

.is-liked {
  background: linear-gradient(135deg, #f87171, #ef4444) !important;
  box-shadow: 0 0 16px rgba(248, 113, 113, 0.3) !important;
}

.comments-count {
  color: var(--text-muted);
  font-size: 14px;
}

.no-data {
  text-align: center;
  padding: 60px;
  color: var(--text-muted);
  font-size: 16px;
}

/* ── 评论区域 ── */
.comments-section {
  margin-top: 18px;
  padding: 4px;
}

.card-header {
  font-weight: 700;
  font-size: 18px;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 8px;
}

.comment-icon { color: var(--accent-cyan); }

.comment-form { margin-bottom: 28px; }

.comments-list { margin-top: 20px; }

.comment-item {
  margin-bottom: 18px;
  padding-bottom: 18px;
  border-bottom: 1px solid rgba(100, 150, 240, 0.08);
}

.comment-item:last-child { border-bottom: none; }

.comment-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.comment-author {
  font-weight: 600;
  color: var(--accent-blue);
  font-size: 14px;
}

.comment-time { color: var(--text-muted); font-size: 12px; }

.comment-text {
  margin: 8px 0;
  color: var(--text-primary);
  line-height: 1.65;
  white-space: pre-wrap;
}

.reply-btn { color: var(--accent-cyan) !important; }

.reply-form {
  margin-left: 28px;
  margin-top: 12px;
  padding: 14px;
  background: rgba(100, 160, 255, 0.04);
  border: 1px solid rgba(100, 150, 240, 0.1);
  border-radius: 12px;
}

.pagination-container {
  margin-top: 30px;
  display: flex;
  justify-content: center;
}

.replies-list {
  margin-left: 36px;
  padding-left: 16px;
  border-left: 2px solid rgba(100, 160, 255, 0.15);
  margin-top: 14px;
}

.reply-item {
  padding: 12px;
  margin-bottom: 10px;
  background: rgba(100, 160, 255, 0.03);
  border: 1px solid rgba(100, 150, 240, 0.08);
  border-radius: 10px;
}

.reply-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}

.reply-author {
  font-weight: 600;
  color: var(--accent-teal);
  font-size: 13px;
}

.reply-time { color: var(--text-muted); font-size: 11px; }

.reply-text {
  margin: 6px 0;
  color: var(--text-secondary);
  line-height: 1.55;
  font-size: 13px;
  white-space: pre-wrap;
}

.no-comments {
  text-align: center;
  padding: 40px;
  color: var(--text-muted);
}

.login-prompt { margin-bottom: 20px; }
</style>
