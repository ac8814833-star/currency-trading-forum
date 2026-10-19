<template>
  <el-container>
    <el-main class="page-shell">
      <div class="form-card surface-card">
        <div class="form-glow"></div>
        <h2 class="form-title gradient-text">发布新文章</h2>
        <p class="form-desc">分享你的外汇交易洞察、汇率分析与市场观点</p>
        <el-form :model="form" label-position="top" @submit.prevent="create">
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
              ✦ 发 布
            </el-button>
          </el-form-item>
        </el-form>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import axios from "../axios";

const router = useRouter();
const loading = ref(false);

const form = ref({ title: "", preview: "", content: "" });

const create = async () => {
  if (!form.value.title.trim()) { ElMessage.warning("请输入标题"); return; }
  if (!form.value.content.trim()) { ElMessage.warning("请输入正文"); return; }
  loading.value = true;
  try {
    const res = await axios.post("/articles", form.value);
    ElMessage.success("发布成功！");
    router.push({ name: "NewsDetail", params: { id: res.data.ID || res.data.id } });
  } catch (err: any) {
    ElMessage.error(err.response?.data?.error || "发布失败，请重试");
  } finally {
    loading.value = false;
  }
};
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
  background: radial-gradient(circle, rgba(91, 156, 245, 0.12), transparent 70%);
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
