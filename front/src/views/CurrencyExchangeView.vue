<template>
  <el-container>
    <el-main class="page-shell">
      <div class="exchange-card surface-card">
        <div class="exchange-glow"></div>
        <h2 class="exchange-title gradient-text">汇率兑换工具</h2>
        <p class="exchange-desc">使用系统内保存的示例汇率换算，非实时交易报价，请以实际牌价为准。</p>

        <div>
          <div class="convert-controls">
            <div class="control-group">
              <label class="control-label">持有货币</label>
              <div class="input-with-unit">
                <el-select v-model="form.from" size="large" placeholder="选择货币" class="currency-select">
                  <el-option v-for="c in currencies" :key="c.code" :label="c.label" :value="c.code" />
                </el-select>
                <el-input-number
                  v-model="form.amount"
                  :min="0"
                  :precision="2"
                  size="large"
                  placeholder="金额"
                  class="amount-input"
                />
              </div>
            </div>

            <div class="swap-icon" @click="swapCurrencies">
              <div class="swap-arrow">⇅</div>
            </div>

            <div class="control-group">
              <label class="control-label">目标货币</label>
              <div class="input-with-unit">
                <el-select v-model="form.to" size="large" placeholder="选择货币" class="currency-select">
                  <el-option v-for="c in allCurrencies" :key="c.code" :label="c.label" :value="c.code" />
                </el-select>
                <el-input
                  :model-value="convertedResult"
                  size="large"
                  readonly
                  placeholder="转换结果"
                  class="result-input"
                />
              </div>
            </div>

            <div class="rate-display" v-if="currentRate">
              <span class="rate-label">当前汇率</span>
              <span class="rate-value">1 {{ form.from.toUpperCase() }} = {{ currentRate }} {{ form.to.toUpperCase() }}</span>
              <span class="rate-time">更新于 {{ rateTime }}</span>
            </div>
          </div>

          <el-button type="primary" size="large" round @click="convert" :loading="loading" class="convert-btn">
            ◎ 开始换算
          </el-button>
        </div>
      </div>

      <div class="rate-table surface-card" v-if="rates && rates.length">
        <div class="table-header">
          <span class="table-icon">◆</span>
          <span class="table-title">系统汇率参考表</span>
          <span class="table-count">共 {{ rates.length }} 种货币</span>
        </div>
        <el-table :data="rates" stripe style="width: 100%; border-radius: 12px" :header-cell-style="{ background: 'rgba(14,22,52,0.5)', color: 'var(--text-secondary)' }">
          <el-table-column prop="currency" label="货币代码" width="120" />
          <el-table-column prop="name" label="货币名称" />
          <el-table-column prop="rate" label="中间价 (CNY)" sortable>
            <template #default="{ row }">
              <span class="rate-value-cell">{{ row.rate }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="timestamp" label="更新时间" width="180">
            <template #default="{ row }">
              <span class="rate-time-cell">{{ formatTime(row.timestamp) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { ElMessage } from "element-plus";
import axios from "../axios";
const loading = ref(false);

const form = ref({ from: "usd", to: "cny", amount: 100 });
const converted = ref<string | null>(null);
const convertedInput = ref<{ amount: number; from: string; to: string } | null>(null);
const currentRate = ref<number | null>(null);
const rateTime = ref("");

const rates = ref<any[]>([]);

const currencies = [
  { code: "usd", label: "USD - 美元" },
  { code: "eur", label: "EUR - 欧元" },
  { code: "gbp", label: "GBP - 英镑" },
  { code: "jpy", label: "JPY - 日元" },
  { code: "hkd", label: "HKD - 港币" },
  { code: "aud", label: "AUD - 澳元" },
  { code: "cad", label: "CAD - 加元" },
  { code: "chf", label: "CHF - 瑞郎" },
  { code: "sgd", label: "SGD - 新加坡元" },
  { code: "krw", label: "KRW - 韩元" },
];

const allCurrencies = [
  ...currencies,
  { code: "cny", label: "CNY - 人民币" },
];

const convertedResult = computed(() => {
  if (!converted.value || !convertedInput.value) return "";
  return `${convertedInput.value.amount} ${convertedInput.value.from.toUpperCase()} = ${converted.value} ${convertedInput.value.to.toUpperCase()}`;
});

watch(() => [form.value.amount, form.value.from, form.value.to], () => {
  converted.value = null;
  convertedInput.value = null;
  currentRate.value = null;
});

const convert = async () => {
  if (!form.value.amount || form.value.amount <= 0) {
    ElMessage.warning("请输入有效金额");
    return;
  }
  if (form.value.from === form.value.to) {
    ElMessage.warning("持有货币和目标货币不能相同");
    return;
  }
  loading.value = true;
  const input = { ...form.value };
  try {
    const response = await axios.get(`/currency/convert`, {
      params: input,
    });
    if (response.data) {
      if (input.amount !== form.value.amount || input.from !== form.value.from || input.to !== form.value.to) return;
      convertedInput.value = input;
      converted.value = response.data.converted || "0";
      currentRate.value = response.data.rate || null;
      rateTime.value = response.data.time || new Date().toLocaleString();
    }
  } catch (err: any) {
    ElMessage.error(err.response?.data?.error || "换算失败，请重试");
  } finally {
    loading.value = false;
  }
};

const swapCurrencies = () => {
  const tmp = form.value.from;
  form.value.from = form.value.to;
  form.value.to = tmp;
  converted.value = null;
  currentRate.value = null;
};

const fetchRates = async () => {
  try {
    const response = await axios.get("/currency/rates");
    rates.value = response.data || response.data.data || [];
  } catch (err) {
    console.error("Failed to fetch rates:", err);
  }
};

const formatTime = (timestamp: string) => {
  if (!timestamp) return "";
  return new Date(timestamp).toLocaleString("zh-CN");
};

// auto-fetch rates
onMounted(fetchRates);
</script>

<style scoped>
.exchange-card {
  position: relative;
  max-width: 800px;
  margin: 0 auto 24px;
  padding: 36px;
  overflow: hidden;
}

.exchange-glow {
  position: absolute;
  top: -80px;
  left: -80px;
  width: 250px;
  height: 250px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(45, 212, 191, 0.1), transparent 70%);
  pointer-events: none;
}

.exchange-title {
  margin: 0;
  font-size: 28px;
}

.exchange-desc {
  margin: 8px 0 28px;
  color: var(--text-muted);
  font-size: 14px;
}

.converter-panel {
  margin-top: 8px;
}

.convert-controls {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.control-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.control-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.input-with-unit {
  display: flex;
  gap: 10px;
}

.currency-select { width: 220px; }

.amount-input { flex: 1; min-width: 0; }

.swap-icon {
  display: flex;
  justify-content: center;
  padding: 4px 0;
}

.swap-arrow {
  width: 36px;
  height: 36px;
  line-height: 36px;
  text-align: center;
  border-radius: 50%;
  background: rgba(100, 160, 255, 0.1);
  border: 1px solid rgba(100, 160, 255, 0.2);
  color: var(--accent-cyan);
  cursor: pointer;
  transition: all 0.3s ease;
  font-size: 16px;
}

.swap-arrow:hover {
  background: rgba(100, 160, 255, 0.2);
  box-shadow: 0 0 12px rgba(34, 211, 238, 0.2);
}

.target-unit {
  font-size: 13px;
  color: var(--text-muted);
}

.result-input {
  font-weight: 600;
}

.rate-display {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  background: rgba(45, 212, 191, 0.06);
  border: 1px solid rgba(45, 212, 191, 0.15);
  border-radius: 12px;
  flex-wrap: wrap;
}

.rate-label { font-size: 13px; color: var(--text-muted); }
.rate-value { font-size: 15px; font-weight: 600; color: var(--accent-teal); }
.rate-time { font-size: 12px; color: var(--text-muted); margin-left: auto; }

.convert-btn {
  width: 100%;
  margin-top: 20px;
}

/* ── Rate Table ── */
.rate-table {
  max-width: 800px;
  margin: 0 auto;
  padding: 24px;
}

.table-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid rgba(100, 150, 240, 0.1);
}

.table-icon { color: var(--accent-cyan); font-size: 14px; }

.table-title {
  font-weight: 700;
  font-size: 16px;
  color: var(--text-primary);
}

.table-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-muted);
}

.rate-value-cell {
  font-weight: 600;
  color: var(--accent-teal);
}

.rate-time-cell {
  color: var(--text-muted);
  font-size: 13px;
}

@media (max-width: 600px) {
  .exchange-card { padding: 22px 16px; }
  .exchange-title { font-size: 24px; }
  .input-with-unit { flex-direction: column; }
  .currency-select, .amount-input, .result-input { width: 100%; }
  .rate-table { padding: 18px 12px; overflow-x: auto; }
  .rate-table :deep(.el-table) { min-width: 580px; }
}
</style>
