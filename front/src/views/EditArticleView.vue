<template>
  <el-container>
    <el-main class="page-shell">
      <div class="form-card surface-card">
        <div class="form-glow"></div>
        <h2 class="form-title gradient-text">编辑文章</h2>
        <p class="form-desc">修改文章内容后提交更新</p>
        <el-form :model="form" label-position="top" @submit.prevent="update">
          <el-form-item label="标题">
            <el-input v-model="form.title" placeholder="请输入标题" maxlength="100" show-word-limit size="large" />
          </el-form-item>
          <el-form-item label="摘要">
            <el-input v-model="form.preview" type="textarea" :rows="2" placeholder="写一段简短摘要..." maxlength="200" show-word-limit />
          </el-form-item>
          <el-form-item label="正文">
            <el-input v-model="form.content" type="textarea" :rows="10" placeholder="在这里写下你的观点与分析..." maxlength="5000" show-word-limit />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" native-type="submit" size="large" round :loading="loading" class="form-btn">
              ✦ 保存更新
            </el-button>
          </el-form-item>
        </el-form>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import axios from "../axios";
import type { Article } from "../types/Article";

const route = useRoute();
const router = useRouter();
const loading = ref(false);

const { id } = route.params;
const form = ref({ title: "", preview: "", content: "" });

const fetchArticle = async () => {
  try {
    const response = await axios.get<Article>(`/articles/${id}`);
    const article = response.data;
    form.value.title = article.Title;
    form.value.preview = article.Preview;
    form.value.content = article.Content;
  } catch (error: any) {
    ElMessage.error("加载文章失败");
    router.push({ name: "News" });
  }
};

const update = async () => {
  if (!form.value.title.trim()) { ElMessage.warning("请输入标题"); return; }
  if (!form.value.content.trim()) { ElMessage.warning("请输入正文"); return; }
  loading.value = true;
  try {
    await axios.put(`/articles/${id}`, form.value);
    ElMessage.success("更新成功！");
    router.push({ name: "NewsDetail", params: { id: id as string } });
  } catch (err: any) {
    ElMessage.error(err.response?.data?.error || "更新失败");
  } finally {
    loading.value = false;
  }
};

onMounted(fetchArticle);
</script>

<style scoped>
.form-card {
  position: relative;
  max-width: 760px;
  margin: 0 auto;
  padding: 36px;
  overflow: hidden;
}

.form-glow {
  position: absolute;
  top: -80px;
  right: -80px;
  width: 260px;
  height: 260px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(167, 139, 250, 0.12), transparent 70%);
  pointer-events: none;
}

.form-title {
  margin: 0 0 6px;
  font-size: 28px;
}

.form-desc {
  margin: 0 0 28px;
  color: var(--text-muted);
  font-size: 14px;
}

.form-btn {
  width: 100%;
  margin-top: 8px;
}
</style>
