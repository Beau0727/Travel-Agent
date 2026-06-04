<script setup lang="ts">
import { message } from "ant-design-vue";
import { onMounted, ref, watch } from "vue";
import { Archive, ArrowRight, Clock3, RefreshCcw, Search, Trash2 } from "@lucide/vue";

import { deleteTrip, getTripDetail, listTrips } from "../services/api";
import type { Itinerary, TripSummaryItem } from "../types";

const props = defineProps<{
  active: boolean;
}>();

const emit = defineEmits<{
  openTrip: [itinerary: Itinerary];
}>();

const loading = ref(false);
const items = ref<TripSummaryItem[]>([]);
const deletingTripId = ref("");

function formatTime(value?: string | null): string {
  if (!value) {
    return "未记录";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

async function loadTrips() {
  loading.value = true;
  try {
    const response = await listTrips();
    items.value = response.items;
  } catch (error) {
    console.error(error);
    message.error("历史列表加载失败。");
  } finally {
    loading.value = false;
  }
}

async function openTrip(tripId: string) {
  try {
    const response = await getTripDetail(tripId);
    emit("openTrip", response.itinerary);
    message.success("已加载保存的行程。");
  } catch (error) {
    console.error(error);
    message.error("读取行程详情失败。");
  }
}

async function removeTrip(tripId: string) {
  const confirmed = window.confirm("确定要删除这条已保存行程吗？删除后无法恢复。");
  if (!confirmed) {
    return;
  }

  deletingTripId.value = tripId;
  try {
    await deleteTrip(tripId);
    items.value = items.value.filter((item) => item.trip_id !== tripId);
    message.success("行程已删除。");
  } catch (error) {
    console.error(error);
    message.error("删除行程失败。");
  } finally {
    deletingTripId.value = "";
  }
}

onMounted(() => {
  if (props.active) {
    void loadTrips();
  }
});

watch(
  () => props.active,
  (active) => {
    if (active) {
      void loadTrips();
    }
  }
);
</script>

<template>
  <section class="history-library">
    <div class="library-hero surface-panel">
      <div>
        <p class="panel-kicker">Archive</p>
        <h2>保存过的旅行路线</h2>
        <p>这里展示已经写入后端数据库的 itinerary，可重新打开、导出或删除。</p>
      </div>
      <button class="command-button" type="button" @click="loadTrips">
        <RefreshCcw :size="17" />
        刷新
      </button>
    </div>

    <div v-if="loading" class="library-state surface-panel">
      <RefreshCcw class="spin" :size="22" />
      正在加载历史列表...
    </div>

    <div v-else-if="items.length === 0" class="library-empty surface-panel">
      <Archive :size="34" />
      <h3>还没有保存的行程</h3>
      <p>生成路线后点击保存，它会出现在这里。</p>
    </div>

    <div v-else class="archive-list">
      <article v-for="item in items" :key="item.trip_id" class="archive-card surface-panel">
        <div class="archive-card__marker">
          <Archive :size="20" />
        </div>

        <div class="archive-card__main">
          <div class="archive-card__title-row">
            <div>
              <h3>{{ item.destination }}</h3>
              <span>{{ item.trip_id }}</span>
            </div>
            <div class="archive-card__time">
              <Clock3 :size="15" />
              {{ formatTime(item.updated_at || item.created_at) }}
            </div>
          </div>

          <p>{{ item.summary }}</p>

          <div class="archive-card__actions">
            <button class="ghost-button" type="button" @click="openTrip(item.trip_id)">
              <Search :size="16" />
              查看详情
            </button>
            <button class="command-button" type="button" @click="openTrip(item.trip_id)">
              打开路线
              <ArrowRight :size="16" />
            </button>
            <button
              class="danger-button"
              :disabled="deletingTripId === item.trip_id"
              type="button"
              @click="removeTrip(item.trip_id)"
            >
              <Trash2 :size="16" />
              {{ deletingTripId === item.trip_id ? "删除中" : "删除" }}
            </button>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
.history-library {
  display: grid;
  gap: 16px;
}

.library-hero {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  padding: 22px;
}

.library-hero h2 {
  margin: 0;
  color: var(--ink);
  font-size: 30px;
  line-height: 1.15;
}

.library-hero p:last-child {
  margin: 10px 0 0;
  color: var(--muted);
  line-height: 1.65;
}

.library-state,
.library-empty {
  min-height: 240px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  padding: 28px;
  color: var(--muted);
  text-align: center;
}

.library-empty h3,
.library-empty p {
  margin: 0;
}

.library-empty h3 {
  color: var(--ink);
  font-size: 22px;
}

.archive-list {
  display: grid;
  gap: 12px;
}

.archive-card {
  display: grid;
  grid-template-columns: 54px 1fr;
  gap: 16px;
  padding: 16px;
}

.archive-card__marker {
  width: 54px;
  height: 54px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: var(--ink);
  color: #fffdf7;
}

.archive-card__main {
  min-width: 0;
}

.archive-card__title-row {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 16px;
}

.archive-card h3 {
  margin: 0;
  color: var(--ink);
  font-size: 24px;
}

.archive-card__title-row span {
  display: block;
  margin-top: 5px;
  color: var(--muted);
  font-size: 12px;
  word-break: break-all;
}

.archive-card__time {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  white-space: nowrap;
  color: var(--muted-strong);
  font-size: 13px;
  font-weight: 800;
}

.archive-card p {
  margin: 12px 0 0;
  color: var(--muted-strong);
  line-height: 1.68;
}

.archive-card__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
  margin-top: 14px;
}

.spin {
  animation: spin 0.9s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 760px) {
  .library-hero,
  .archive-card__title-row {
    align-items: stretch;
    flex-direction: column;
  }

  .archive-card {
    grid-template-columns: 1fr;
  }

  .archive-card__time {
    white-space: normal;
  }
}
</style>
