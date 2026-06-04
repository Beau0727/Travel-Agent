<script setup lang="ts">
import axios from "axios";
import { computed, reactive, ref } from "vue";
import { message } from "ant-design-vue";
import {
  CalendarDays,
  Check,
  Clock3,
  Coins,
  Hotel,
  LoaderCircle,
  Map,
  MapPinned,
  NotebookPen,
  SendHorizontal,
  SlidersHorizontal,
  Sparkles,
  Users,
  Utensils,
} from "@lucide/vue";

import { generateTrip } from "../services/api";
import type { Itinerary, TripRequestPayload } from "../types";

const emit = defineEmits<{
  generated: [itinerary: Itinerary];
}>();

const preferenceOptions = ["自然风景", "拍照", "美食", "古镇", "休闲", "亲子", "博物馆", "徒步"];
const dietaryOptions = ["少辣", "不吃香菜", "不吃葱", "素食友好", "海鲜优先"];

const destinationPresets = [
  { name: "大理", tone: "洱海与古城", budget: 3200, pace: "轻松" },
  { name: "成都", tone: "烟火与慢生活", budget: 2800, pace: "适中" },
  { name: "厦门", tone: "海岛与街巷", budget: 3600, pace: "轻松" },
  { name: "三亚", tone: "海滩与度假", budget: 5200, pace: "舒适" },
];

function formatDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

const today = new Date();
const todayPlus2 = new Date(today);
todayPlus2.setDate(todayPlus2.getDate() + 2);

const formState = reactive({
  destination: "大理",
  startDate: formatDate(today),
  endDate: formatDate(todayPlus2),
  travelers: 2,
  budget: 3200,
  hotelLevel: "舒适型",
  pace: "轻松",
  preferences: ["自然风景", "拍照", "美食"],
  dietaryPreferences: ["少辣"],
  notes: "不想太早起床，希望安排一个适合看日落的地点。",
});

const isSubmitting = ref(false);

const dayCount = computed(() => {
  const start = new Date(formState.startDate);
  const end = new Date(formState.endDate);
  const diff = end.getTime() - start.getTime();
  return Number.isNaN(diff) ? 0 : Math.max(Math.floor(diff / 86400000) + 1, 0);
});

const perPersonBudget = computed(() => {
  if (!formState.travelers) {
    return 0;
  }

  return Math.round(formState.budget / formState.travelers);
});

const dateRangeText = computed(() => {
  if (!formState.startDate || !formState.endDate) {
    return "待定";
  }

  return `${formState.startDate} 至 ${formState.endDate}`;
});

function applyPreset(preset: (typeof destinationPresets)[number]) {
  formState.destination = preset.name;
  formState.budget = preset.budget;
  formState.pace = preset.pace;
}

function toggleOption(target: string[], value: string) {
  const index = target.indexOf(value);
  if (index >= 0) {
    target.splice(index, 1);
    return;
  }

  target.push(value);
}

async function handleSubmit() {
  const payload: TripRequestPayload = {
    destination: formState.destination,
    start_date: formState.startDate,
    end_date: formState.endDate,
    travelers: formState.travelers,
    budget: formState.budget,
    preferences: formState.preferences,
    pace: formState.pace,
    dietary_preferences: formState.dietaryPreferences,
    hotel_level: formState.hotelLevel,
    special_notes: formState.notes,
  };

  isSubmitting.value = true;
  try {
    const itinerary = await generateTrip(payload);
    message.success("行程生成成功，已切换到路线板。");
    emit("generated", itinerary);
  } catch (error) {
    console.error(error);
    if (axios.isAxiosError(error)) {
      if (error.code === "ECONNABORTED") {
        message.error("行程生成超时，模型返回较慢，请稍后再试。");
      } else if (error.response) {
        message.error(`行程生成失败：后端返回 ${error.response.status}。`);
      } else {
        message.error("行程生成失败，请检查前端到后端的连接。");
      }
    } else {
      message.error("行程生成失败，请检查后端地址或服务状态。");
    }
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <section class="planner-workbench">
    <div class="planner-main surface-panel">
      <div class="panel-header">
        <div>
          <p class="panel-kicker">Trip Request</p>
          <h2 class="panel-title">把旅行约束整理成一份路线任务</h2>
          <p class="panel-copy">目的地、预算、偏好和节奏会一起发给后端生成 itinerary。</p>
        </div>
        <span class="metric-chip">
          <CalendarDays :size="16" />
          {{ dayCount }} 天
        </span>
      </div>

      <div class="form-section">
        <div class="section-label">
          <MapPinned :size="18" />
          <span>目的地与日期</span>
        </div>

        <div class="preset-strip">
          <button
            v-for="preset in destinationPresets"
            :key="preset.name"
            class="preset-card"
            :class="{ 'preset-card--active': formState.destination === preset.name }"
            type="button"
            @click="applyPreset(preset)"
          >
            <strong>{{ preset.name }}</strong>
            <span>{{ preset.tone }}</span>
          </button>
        </div>

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="10">
            <label class="field-label">
              <Map :size="15" />
              目的地城市
            </label>
            <a-input v-model:value="formState.destination" placeholder="请输入目的地" />
          </a-col>
          <a-col :xs="24" :sm="12" :lg="7">
            <label class="field-label">
              <CalendarDays :size="15" />
              开始日期
            </label>
            <a-input v-model:value="formState.startDate" />
          </a-col>
          <a-col :xs="24" :sm="12" :lg="7">
            <label class="field-label">
              <CalendarDays :size="15" />
              结束日期
            </label>
            <a-input v-model:value="formState.endDate" />
          </a-col>
        </a-row>
      </div>

      <div class="form-section">
        <div class="section-label">
          <SlidersHorizontal :size="18" />
          <span>预算与节奏</span>
        </div>

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :sm="12" :xl="6">
            <label class="field-label">
              <Users :size="15" />
              出行人数
            </label>
            <a-input-number v-model:value="formState.travelers" :min="1" style="width: 100%" />
          </a-col>
          <a-col :xs="24" :sm="12" :xl="6">
            <label class="field-label">
              <Coins :size="15" />
              总预算
            </label>
            <a-input-number v-model:value="formState.budget" :min="0" style="width: 100%" />
          </a-col>
          <a-col :xs="24" :sm="12" :xl="6">
            <label class="field-label">
              <Clock3 :size="15" />
              行程节奏
            </label>
            <a-select
              v-model:value="formState.pace"
              :options="[
                { label: '轻松', value: '轻松' },
                { label: '适中', value: '适中' },
                { label: '紧凑', value: '紧凑' },
                { label: '舒适', value: '舒适' }
              ]"
            />
          </a-col>
          <a-col :xs="24" :sm="12" :xl="6">
            <label class="field-label">
              <Hotel :size="15" />
              住宿偏好
            </label>
            <a-select
              v-model:value="formState.hotelLevel"
              :options="[
                { label: '舒适型', value: '舒适型' },
                { label: '高档型', value: '高档型' },
                { label: '经济型', value: '经济型' }
              ]"
            />
          </a-col>
        </a-row>
      </div>

      <div class="form-section">
        <div class="section-label">
          <Sparkles :size="18" />
          <span>兴趣标签</span>
        </div>
        <div class="tag-cloud">
          <button
            v-for="option in preferenceOptions"
            :key="option"
            class="toggle-tag"
            :class="{ 'toggle-tag--active': formState.preferences.includes(option) }"
            type="button"
            @click="toggleOption(formState.preferences, option)"
          >
            <Check v-if="formState.preferences.includes(option)" :size="14" />
            {{ option }}
          </button>
        </div>
      </div>

      <div class="form-section">
        <div class="section-label">
          <Utensils :size="18" />
          <span>饮食偏好</span>
        </div>
        <div class="tag-cloud">
          <button
            v-for="option in dietaryOptions"
            :key="option"
            class="toggle-tag toggle-tag--food"
            :class="{ 'toggle-tag--active': formState.dietaryPreferences.includes(option) }"
            type="button"
            @click="toggleOption(formState.dietaryPreferences, option)"
          >
            <Check v-if="formState.dietaryPreferences.includes(option)" :size="14" />
            {{ option }}
          </button>
        </div>
      </div>

      <div class="form-section">
        <div class="section-label">
          <NotebookPen :size="18" />
          <span>额外要求</span>
        </div>
        <a-textarea
          v-model:value="formState.notes"
          :rows="4"
          placeholder="输入想要保留的偏好、节奏和备注"
        />
      </div>
    </div>

    <aside class="briefing-panel">
      <div class="briefing-card briefing-card--dark">
        <p class="briefing-kicker">Live Brief</p>
        <h2>{{ formState.destination || "未设置目的地" }}</h2>
        <p>{{ dateRangeText }}</p>
        <div class="briefing-grid">
          <span>
            <strong>{{ formState.travelers }}</strong>
            人
          </span>
          <span>
            <strong>{{ dayCount }}</strong>
            天
          </span>
          <span>
            <strong>¥{{ perPersonBudget }}</strong>
            / 人
          </span>
        </div>
      </div>

      <div class="briefing-card">
        <div class="briefing-row">
          <span>路线节奏</span>
          <strong>{{ formState.pace }}</strong>
        </div>
        <div class="briefing-row">
          <span>住宿类型</span>
          <strong>{{ formState.hotelLevel }}</strong>
        </div>
        <div class="briefing-row">
          <span>兴趣数量</span>
          <strong>{{ formState.preferences.length }} 项</strong>
        </div>
        <div class="briefing-row">
          <span>饮食限制</span>
          <strong>{{ formState.dietaryPreferences.length }} 项</strong>
        </div>
      </div>

      <button
        class="generate-button"
        :disabled="isSubmitting"
        type="button"
        @click="handleSubmit"
      >
        <LoaderCircle v-if="isSubmitting" class="spin" :size="18" />
        <SendHorizontal v-else :size="18" />
        {{ isSubmitting ? "正在生成路线" : "生成旅行路线" }}
      </button>
    </aside>
  </section>
</template>

<style scoped>
.planner-workbench {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 360px;
  gap: 18px;
  align-items: start;
}

.planner-main {
  padding: 22px;
}

.form-section {
  padding: 20px 0;
  border-bottom: 1px solid var(--line);
}

.form-section:last-child {
  border-bottom: 0;
  padding-bottom: 0;
}

.section-label {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  margin-bottom: 14px;
  color: var(--ink);
  font-weight: 900;
}

.preset-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}

.preset-card {
  display: grid;
  gap: 5px;
  min-height: 76px;
  padding: 13px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface-2);
  color: var(--text);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.16s ease, transform 0.16s ease, background 0.16s ease;
}

.preset-card strong {
  color: var(--ink);
  font-size: 18px;
}

.preset-card span {
  color: var(--muted);
  font-size: 13px;
}

.preset-card:hover {
  transform: translateY(-1px);
  border-color: rgba(15, 118, 110, 0.45);
}

.preset-card--active {
  background: rgba(15, 118, 110, 0.11);
  border-color: rgba(15, 118, 110, 0.58);
}

.tag-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.toggle-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 36px;
  padding: 8px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: #fffdf7;
  color: var(--muted-strong);
  font-weight: 800;
  cursor: pointer;
}

.toggle-tag--active {
  border-color: rgba(15, 118, 110, 0.6);
  background: var(--accent);
  color: #fffdf7;
}

.toggle-tag--food.toggle-tag--active {
  border-color: rgba(192, 86, 33, 0.62);
  background: var(--accent-2);
}

.briefing-panel {
  position: sticky;
  top: 100px;
  display: grid;
  gap: 14px;
}

.briefing-card {
  padding: 18px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: rgba(255, 253, 247, 0.92);
  box-shadow: var(--soft-shadow);
}

.briefing-card--dark {
  background: var(--ink);
  color: #fffdf7;
  border-color: rgba(31, 41, 51, 0.2);
}

.briefing-kicker {
  margin: 0 0 12px;
  color: #93d8d0;
  font-size: 12px;
  font-weight: 900;
  text-transform: uppercase;
}

.briefing-card h2 {
  margin: 0;
  font-size: 32px;
  line-height: 1.1;
}

.briefing-card p {
  margin: 10px 0 0;
  color: rgba(255, 253, 247, 0.72);
}

.briefing-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 18px;
}

.briefing-grid span {
  display: grid;
  gap: 4px;
  padding: 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 253, 247, 0.72);
  font-size: 12px;
}

.briefing-grid strong {
  color: #fffdf7;
  font-size: 18px;
}

.briefing-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 11px 0;
  border-bottom: 1px solid var(--line);
}

.briefing-row:first-child {
  padding-top: 0;
}

.briefing-row:last-child {
  border-bottom: 0;
  padding-bottom: 0;
}

.briefing-row span {
  color: var(--muted);
}

.briefing-row strong {
  color: var(--ink);
}

.generate-button {
  min-height: 54px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  width: 100%;
  border: 0;
  border-radius: 8px;
  background: var(--accent-2);
  color: #fffdf7;
  font-weight: 950;
  cursor: pointer;
  box-shadow: 0 16px 30px rgba(192, 86, 33, 0.26);
}

.generate-button:disabled {
  cursor: wait;
  opacity: 0.7;
}

.spin {
  animation: spin 0.9s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1160px) {
  .planner-workbench {
    grid-template-columns: 1fr;
  }

  .briefing-panel {
    position: static;
    grid-template-columns: 1fr 1fr;
  }

  .generate-button {
    grid-column: 1 / -1;
  }
}

@media (max-width: 760px) {
  .planner-main {
    padding: 16px;
  }

  .panel-header {
    align-items: start;
    flex-direction: column;
  }

  .preset-strip,
  .briefing-panel {
    grid-template-columns: 1fr;
  }

  .briefing-grid {
    grid-template-columns: 1fr;
  }
}
</style>
