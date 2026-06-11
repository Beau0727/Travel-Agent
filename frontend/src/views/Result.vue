<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { message } from "ant-design-vue";
import {
  Archive,
  ArrowLeft,
  CalendarDays,
  CloudRain,
  CloudSun,
  Download,
  ExternalLink,
  Hotel,
  MapPinned,
  PencilLine,
  Route,
  Save,
  ShieldCheck,
  Sparkles,
  Ticket,
  TramFront,
  Utensils,
  Wallet,
  WandSparkles,
} from "@lucide/vue";

import AmapTripMap from "../components/AmapTripMap.vue";
import {
  editTrip,
  fetchWeatherForecast,
  getMarkdownExportUrl,
  saveTrip,
} from "../services/api";
import type {
  DayPlan,
  EvidenceClaim,
  EvidenceSource,
  Itinerary,
  TransportItem,
  TripEditMessage,
  WeatherForecastResponse,
} from "../types";

const props = defineProps<{
  itinerary: Itinerary | null;
}>();

const emit = defineEmits<{
  backHome: [];
  viewHistory: [];
  updated: [itinerary: Itinerary];
}>();

const saving = ref(false);
const exportingMarkdown = ref(false);
const editing = ref(false);
const editScope = ref("day_1");
const editInstruction = ref("这一天节奏更轻松一点，减少固定安排，保留核心景点。");
const weatherLoading = ref(false);
const weatherError = ref("");
const weather = ref<WeatherForecastResponse | null>(null);
const failedImageUrls = ref<Set<string>>(new Set());

function formatShortDate(dateText?: string | null): string {
  if (!dateText) {
    return "待定";
  }

  const parts = dateText.split("-");
  if (parts.length !== 3) {
    return dateText;
  }

  return `${parts[1]}-${parts[2]}`;
}

function formatWeatherDate(dateText?: string | null, week?: string | null): string {
  const weekdayMap: Record<string, string> = {
    "1": "周一",
    "2": "周二",
    "3": "周三",
    "4": "周四",
    "5": "周五",
    "6": "周六",
    "7": "周日",
  };
  const weekday = week ? weekdayMap[week] || `周${week}` : "";
  return [formatShortDate(dateText), weekday].filter(Boolean).join(" ");
}

function money(value?: number | null): string {
  return `¥${Math.round(value ?? 0)}`;
}

function joinPlaces(item: { from_place?: string | null; to_place?: string | null }): string {
  return [item.from_place, item.to_place].filter(Boolean).join(" → ") || "路线待补充";
}

function hasUsableImage(url?: string | null): url is string {
  return Boolean(url && !failedImageUrls.value.has(url));
}

function markImageFailed(url?: string | null) {
  if (!url) {
    return;
  }
  failedImageUrls.value = new Set([...failedImageUrls.value, url]);
}

const dateRange = computed(() => {
  if (!props.itinerary?.days.length) {
    return "日期待定";
  }

  const firstDay = props.itinerary.days[0]?.date || "待定";
  const lastDay = props.itinerary.days[props.itinerary.days.length - 1]?.date || "待定";
  return `${firstDay} 至 ${lastDay}`;
});

const totalSpotCount = computed(() => {
  return props.itinerary?.days.reduce((sum, day) => sum + day.spots.length, 0) ?? 0;
});

const totalMealCount = computed(() => {
  return props.itinerary?.days.reduce((sum, day) => sum + day.meals.length, 0) ?? 0;
});

const budgetItems = computed(() => {
  if (!props.itinerary) {
    return [];
  }

  const budget = props.itinerary.budget_breakdown;
  return [
    { label: "景点门票", value: budget.tickets, icon: Ticket },
    { label: "酒店住宿", value: budget.hotel, icon: Hotel },
    { label: "餐饮费用", value: budget.meals, icon: Utensils },
    { label: "交通费用", value: budget.transport, icon: TramFront },
  ];
});

const dayBudgetItems = computed(() => {
  if (!props.itinerary) {
    return [];
  }

  return props.itinerary.days.map((day) => {
    const tickets = day.spots.reduce((sum, spot) => sum + (spot.estimated_cost ?? 0), 0);
    const meals = day.meals.reduce((sum, meal) => sum + (meal.estimated_cost ?? 0), 0);
    const transport = day.transport.reduce((sum, item) => sum + (item.estimated_cost ?? 0), 0);
    const hotel = day.hotel?.estimated_cost ?? 0;
    const total = tickets + meals + transport + hotel;

    return {
      key: day.day_index,
      title: `第 ${day.day_index} 天`,
      subtitle: day.theme || "未命名主题",
      tickets,
      meals,
      transport,
      hotel,
      total,
    };
  });
});

const mapPoints = computed(() => {
  if (!props.itinerary) {
    return [];
  }

  return props.itinerary.days.flatMap((day) => {
    const spotPoints = day.spots.map((spot) => ({
      key: `${day.day_index}-${spot.name}`,
      kind: "spot" as const,
      dayIndex: day.day_index,
      date: day.date || "待定",
      theme: day.theme || "未命名主题",
      name: spot.name,
      address: spot.address || spot.location || "待补充",
      latitude: spot.latitude,
      longitude: spot.longitude,
      poiId: spot.poi_id,
      imageUrl: spot.image_url,
      description: spot.description || "暂无说明",
    }));

    const mealPoints = day.meals.map((meal) => ({
      key: `${day.day_index}-meal-${meal.name}`,
      kind: "meal" as const,
      dayIndex: day.day_index,
      date: day.date || "待定",
      theme: day.theme || "未命名主题",
      name: meal.name,
      address: meal.address || meal.location || "待补充",
      latitude: meal.latitude,
      longitude: meal.longitude,
      poiId: meal.poi_id,
      imageUrl: meal.image_url,
      description: meal.notes || "暂无说明",
    }));

    return [...spotPoints, ...mealPoints];
  });
});

const routePolylines = computed(() => {
  if (!props.itinerary) {
    return [];
  }

  return props.itinerary.days.flatMap((day) =>
    day.transport
      .map((item) => item.polyline?.trim())
      .filter((polyline): polyline is string => Boolean(polyline))
  );
});

const routedLegCount = computed(() => {
  if (!props.itinerary) {
    return 0;
  }

  return props.itinerary.days.reduce(
    (sum, day) => sum + day.transport.filter((item) => item.route_status === "ok").length,
    0
  );
});

const editMessages = computed<TripEditMessage[]>(() => {
  return props.itinerary?.edit_messages ?? [];
});

const lastChangeSummary = computed(() => {
  return props.itinerary?.last_change_summary ?? [];
});

const editIssues = computed(() => {
  return props.itinerary?.edit_issues ?? [];
});

const editRevisionCount = computed(() => {
  return props.itinerary?.edit_revisions?.length ?? 0;
});

const evidence = computed(() => props.itinerary?.evidence ?? null);

const evidenceClaims = computed(() => {
  return evidence.value?.claims?.slice(0, 6) ?? [];
});

const evidenceSources = computed(() => {
  return evidence.value?.sources?.slice(0, 5) ?? [];
});

const evidenceWarnings = computed(() => {
  return evidence.value?.warnings ?? [];
});

const evidenceSummary = computed(() => {
  return evidence.value?.summary ?? [];
});

const evidenceVerificationSummary = computed(() => {
  return evidence.value?.verification_summary ?? [];
});

const evidenceStats = computed(() => {
  const report = evidence.value;
  const claims = report?.claims ?? [];
  const sources = report?.sources ?? [];
  return {
    sources: sources.length,
    claims: claims.length,
    review: claims.filter((claim) => claim.requires_review).length,
    officialCross: claims.filter(
      (claim) => claim.verification_status === "official_cross_verified"
    ).length,
    officialSources: sources.filter((source) => source.source_type === "official").length,
    operationalSources: sources.filter(
      (source) =>
        source.source_type === "map_or_local_service" || source.source_type === "ticketing"
    ).length,
  };
});

const technicalTipKeywords = ["LLM", "RAG", "LangChain", "Chroma", "演示", "测试", "规则", "模型", "源码"];
const rainWeatherKeywords = ["雨", "阵雨", "雷阵雨", "小雨", "中雨", "大雨", "暴雨"];
const sunnyTipKeywords = ["防晒", "太阳", "日照", "晒"];

const weatherText = computed(() => {
  if (!weather.value) {
    return "";
  }

  return weather.value.days
    .map((day) => `${day.day_weather || ""}${day.night_weather || ""}`)
    .join(" ");
});

const hasRainyWeather = computed(() => {
  return rainWeatherKeywords.some((keyword) => weatherText.value.includes(keyword));
});

const displayTips = computed(() => {
  if (!props.itinerary) {
    return [];
  }

  const tips = props.itinerary.tips
    .map((tip) => tip.trim())
    .filter(Boolean)
    .filter((tip) => !technicalTipKeywords.some((keyword) => tip.includes(keyword)));

  const weatherAwareTips = hasRainyWeather.value
    ? tips.filter((tip) => !sunnyTipKeywords.some((keyword) => tip.includes(keyword)))
    : tips;

  if (hasRainyWeather.value) {
    weatherAwareTips.push("天气可能有雨，建议随身带伞或轻便雨衣。");
    weatherAwareTips.push("雨天路面湿滑，古镇石板路和海边栈道建议穿防滑鞋。");
  }

  const uniqueTips = Array.from(new Set(weatherAwareTips));
  if (uniqueTips.length) {
    return uniqueTips;
  }

  return [
    `建议根据${props.itinerary.destination}当天实时天气准备雨具或轻薄外套。`,
    "古镇、生态廊道和石板路更适合慢慢走，鞋子尽量选择舒适防滑的款式。",
  ];
});

const editScopeOptions = computed(() => {
  if (!props.itinerary) {
    return [{ label: "全行程", value: "all" }];
  }

  return [
    { label: "全行程", value: "all" },
    ...props.itinerary.days.map((day) => ({
      label: `第 ${day.day_index} 天：${day.theme || "未命名主题"}`,
      value: `day_${day.day_index}`,
    })),
  ];
});

function buildVisibleItinerary(): Itinerary | null {
  if (!props.itinerary) {
    return null;
  }

  return {
    ...props.itinerary,
    tips: displayTips.value,
  };
}

async function loadWeather() {
  if (!props.itinerary?.destination) {
    weather.value = null;
    return;
  }

  weatherLoading.value = true;
  weatherError.value = "";
  try {
    weather.value = await fetchWeatherForecast(props.itinerary.destination);
  } catch (error) {
    console.error(error);
    weather.value = null;
    weatherError.value = "天气信息加载失败。";
  } finally {
    weatherLoading.value = false;
  }
}

watch(
  () => props.itinerary?.destination,
  () => {
    void loadWeather();
  },
  { immediate: true }
);

watch(
  () => props.itinerary?.trip_id,
  () => {
    failedImageUrls.value = new Set();
    const firstDay = props.itinerary?.days[0];
    editScope.value = firstDay ? `day_${firstDay.day_index}` : "all";
  },
  { immediate: true }
);

async function openMarkdownExport() {
  const itineraryToExport = buildVisibleItinerary();
  if (!itineraryToExport) {
    return;
  }

  const exportWindow = window.open("about:blank", "_blank");
  exportingMarkdown.value = true;
  try {
    await saveTrip(itineraryToExport);
    const exportUrl = getMarkdownExportUrl(itineraryToExport.trip_id);
    if (exportWindow) {
      exportWindow.location.href = exportUrl;
    } else {
      window.location.href = exportUrl;
    }
  } catch (error) {
    console.error(error);
    exportWindow?.close();
    message.error("导出 Markdown 前同步当前行程失败。");
  } finally {
    exportingMarkdown.value = false;
  }
}

async function handleSave() {
  const itineraryToSave = buildVisibleItinerary();
  if (!itineraryToSave) {
    return;
  }

  saving.value = true;
  try {
    await saveTrip(itineraryToSave);
    message.success("行程已保存，可以在档案库里查看。");
  } catch (error) {
    console.error(error);
    message.error("保存行程失败。");
  } finally {
    saving.value = false;
  }
}

async function handleEdit() {
  if (!props.itinerary) {
    return;
  }

  const instruction = editInstruction.value.trim();
  if (!instruction) {
    message.warning("请先输入想如何调整行程。");
    return;
  }

  editing.value = true;
  try {
    const updatedItinerary = await editTrip({
      trip_id: props.itinerary.trip_id,
      current_itinerary: props.itinerary,
      user_instruction: instruction,
      edit_scope: editScope.value,
      conversation_id: props.itinerary.edit_conversation_id,
      messages: props.itinerary.edit_messages ?? [],
      preserve_constraints: ["保留预算结构", "保留目的地和旅行日期"],
    });
    emit("updated", updatedItinerary);
    editInstruction.value = "";
    message.success("行程已智能调整。");
  } catch (error) {
    console.error(error);
    message.error("智能调整失败，请稍后再试。");
  } finally {
    editing.value = false;
  }
}

function dayPointCount(day: DayPlan): number {
  return day.spots.length + day.meals.length;
}

function editRoleLabel(role: string): string {
  if (role === "user") {
    return "你";
  }
  if (role === "assistant") {
    return "助手";
  }
  return "工具";
}

function formatEditTime(value?: string | null): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatRouteDistance(item: TransportItem): string {
  if (item.distance_meters && item.distance_meters > 0) {
    const km = item.distance_meters / 1000;
    return `${km.toFixed(km >= 10 ? 1 : 2)} km`;
  }
  if (item.distance_km != null) {
    return `${Number(item.distance_km).toFixed(Number(item.distance_km) >= 10 ? 1 : 2)} km`;
  }
  return "距离待定";
}

function formatRouteDuration(item: TransportItem): string {
  if (item.duration_seconds && item.duration_seconds > 0) {
    const minutes = Math.round(item.duration_seconds / 60);
    if (minutes < 60) {
      return `${minutes} 分钟`;
    }
    return `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分钟`;
  }
  return item.duration || "时长待定";
}

function routeModeText(item: TransportItem): string {
  if (item.route_mode === "walking") {
    return "步行";
  }
  if (item.route_mode === "driving") {
    return "驾车";
  }
  if (item.route_mode === "transit") {
    return "公交";
  }
  return item.mode || "交通";
}

function routeStatusText(item: TransportItem): string {
  if (item.route_status === "ok") {
    return item.route_provider === "amap" ? "高德实算" : "已规划";
  }
  if (item.route_status === "failed") {
    return "规划失败";
  }
  return "估算";
}

function routeMeta(item: TransportItem): string {
  return [
    routeStatusText(item),
    routeModeText(item),
    formatRouteDistance(item),
    formatRouteDuration(item),
    item.route_mode === "walking" ? "免费" : money(item.estimated_cost),
  ].join(" · ");
}

function weatherSourceLabel(source?: string | null): string {
  if (source === "amap") {
    return "高德天气";
  }
  if (source === "demo" || source === "demo_fallback") {
    return "示例天气";
  }
  return "天气数据";
}

function confidenceText(value?: number | null): string {
  return `${Math.round((value ?? 0) * 100)}%`;
}

function evidenceStatusText(status?: string | null): string {
  if (status === "supported") {
    return "已支持";
  }
  if (status === "needs_review") {
    return "待确认";
  }
  return "弱支持";
}

function verificationStatusText(status?: string | null): string {
  const labels: Record<string, string> = {
    official_cross_verified: "官网交叉验证",
    official_supported: "官方支持",
    map_ticketing_cross_verified: "地图票务互证",
    multi_source_supported: "多来源支持",
    single_source: "单一来源",
    unverified: "未验证",
  };
  return status ? labels[status] || status : "未验证";
}

function verificationRoleText(role?: string | null): string {
  const labels: Record<string, string> = {
    official: "官网/官方",
    map: "地图/本地",
    ticketing: "票务/预约",
    travel_platform: "旅行平台",
    community: "社区内容",
    web: "普通网页",
  };
  return role ? labels[role] || role : "来源";
}

function verificationChannelsText(channels?: string[]): string {
  return (channels ?? []).map((channel) => verificationRoleText(channel)).join("、");
}

function claimTypeText(type?: string | null): string {
  const labels: Record<string, string> = {
    attraction: "景点",
    food: "餐饮",
    transport: "交通",
    caution: "注意",
    volatile: "易变",
    general: "综合",
  };
  return type ? labels[type] || type : "综合";
}

function sourceLabel(source: EvidenceSource): string {
  return source.title || source.host || "在线来源";
}

function sourceMeta(source: EvidenceSource): string {
  return [
    verificationRoleText(source.verification_role),
    source.reliability_label || "来源",
    source.source_priority ? `优先级 ${source.source_priority}` : "",
    confidenceText(source.reliability_score),
    source.host,
  ]
    .filter(Boolean)
    .join(" · ");
}

function claimSourceURL(claim: EvidenceClaim): string {
  return claim.official_source_url || claim.source_urls?.[0] || "";
}
</script>

<template>
  <section v-if="itinerary" class="result-dashboard">
    <div class="result-hero">
      <div class="result-hero__copy">
        <p class="panel-kicker">Generated Itinerary</p>
        <h2>{{ itinerary.destination }}</h2>
        <p>{{ itinerary.summary }}</p>
      </div>
      <div class="result-hero__stats">
        <span>
          <CalendarDays :size="16" />
          {{ itinerary.days.length }} 天
        </span>
        <span>
          <MapPinned :size="16" />
          {{ totalSpotCount }} 个点位
        </span>
        <span>
          <Wallet :size="16" />
          {{ money(itinerary.estimated_budget) }}
        </span>
      </div>
    </div>

    <div class="action-rail surface-panel">
      <button class="ghost-button" type="button" @click="$emit('backHome')">
        <ArrowLeft :size="17" />
        重新规划
      </button>
      <button class="command-button" :disabled="saving" type="button" @click="handleSave">
        <Save :size="17" />
        {{ saving ? "保存中" : "保存" }}
      </button>
      <button class="ghost-button" type="button" @click="$emit('viewHistory')">
        <Archive :size="17" />
        档案库
      </button>
      <button
        class="ghost-button"
        :disabled="exportingMarkdown"
        type="button"
        @click="openMarkdownExport"
      >
        <Download :size="17" />
        {{ exportingMarkdown ? "准备中" : "Markdown" }}
      </button>
    </div>

    <div class="dashboard-grid">
      <section class="overview-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Overview</p>
            <h3 class="panel-title">行程概览</h3>
          </div>
          <span class="metric-chip">{{ dateRange }}</span>
        </div>

        <div class="overview-metrics">
          <article>
            <strong>{{ itinerary.days.length }}</strong>
            <span>旅行天数</span>
          </article>
          <article>
            <strong>{{ totalSpotCount }}</strong>
            <span>景点安排</span>
          </article>
          <article>
            <strong>{{ totalMealCount }}</strong>
            <span>餐饮推荐</span>
          </article>
        </div>

        <div class="tips-panel" v-if="displayTips.length">
          <div class="mini-title">
            <Sparkles :size="16" />
            旅行提示
          </div>
          <ul>
            <li v-for="tip in displayTips" :key="tip">{{ tip }}</li>
          </ul>
        </div>
      </section>

      <section class="budget-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Budget</p>
            <h3 class="panel-title">预算结构</h3>
          </div>
          <span class="metric-chip">{{ money(itinerary.estimated_budget) }}</span>
        </div>

        <div class="budget-list">
          <article v-for="item in budgetItems" :key="item.label" class="budget-row">
            <span class="budget-icon">
              <component :is="item.icon" :size="17" />
            </span>
            <span>{{ item.label }}</span>
            <strong>{{ money(item.value) }}</strong>
          </article>
        </div>
      </section>

      <section v-if="evidence" class="evidence-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Evidence</p>
            <h3 class="panel-title">证据与可信度</h3>
          </div>
          <span class="metric-chip">
            <ShieldCheck :size="15" />
            {{ evidenceStats.sources }} 个来源
          </span>
        </div>

        <div class="evidence-grid">
          <div class="evidence-meter">
            <article>
              <strong>{{ evidenceStats.claims }}</strong>
              <span>候选事实</span>
            </article>
            <article>
              <strong>{{ evidenceStats.officialCross }}</strong>
              <span>官网交叉验证</span>
            </article>
            <article>
              <strong>{{ evidenceStats.review }}</strong>
              <span>待确认</span>
            </article>
            <article>
              <strong>{{ evidenceStats.officialSources }}</strong>
              <span>官方来源</span>
            </article>
            <article>
              <strong>{{ evidenceStats.operationalSources }}</strong>
              <span>地图/票务来源</span>
            </article>
            <article>
              <strong>{{ evidenceStats.sources }}</strong>
              <span>在线来源</span>
            </article>
          </div>

          <div
            class="evidence-summary"
            v-if="evidenceSummary.length || evidenceVerificationSummary.length || evidenceWarnings.length"
          >
            <span v-for="item in evidenceSummary" :key="item">{{ item }}</span>
            <span
              v-for="item in evidenceVerificationSummary"
              :key="item"
              class="evidence-verification"
            >
              {{ item }}
            </span>
            <span v-for="item in evidenceWarnings" :key="item" class="evidence-warning">
              {{ item }}
            </span>
          </div>

          <div class="evidence-claims" v-if="evidenceClaims.length">
            <article
              v-for="claim in evidenceClaims"
              :key="claim.id"
              class="evidence-claim"
              :class="{ 'evidence-claim--review': claim.requires_review }"
            >
              <div class="evidence-claim__header">
                <strong>{{ claimTypeText(claim.claim_type) }}</strong>
                <span>{{ evidenceStatusText(claim.status) }} · {{ confidenceText(claim.confidence) }}</span>
              </div>
              <div class="evidence-chip-row">
                <span
                  class="evidence-chip"
                  :class="{ 'evidence-chip--strong': claim.verification_status === 'official_cross_verified' }"
                >
                  {{ verificationStatusText(claim.verification_status) }}
                </span>
                <span v-if="claim.verification_channels?.length" class="evidence-chip">
                  {{ verificationChannelsText(claim.verification_channels) }}
                </span>
              </div>
              <p>{{ claim.claim }}</p>
              <small v-if="claim.verification_summary">{{ claim.verification_summary }}</small>
              <small v-if="claim.reason">{{ claim.reason }}</small>
              <a
                v-if="claimSourceURL(claim)"
                :href="claimSourceURL(claim)"
                target="_blank"
                rel="noreferrer"
              >
                <ExternalLink :size="13" />
                来源
              </a>
            </article>
          </div>

          <div class="evidence-sources" v-if="evidenceSources.length">
            <a
              v-for="source in evidenceSources"
              :key="source.id"
              :href="source.url || '#'"
              :class="{ 'evidence-source--disabled': !source.url }"
              target="_blank"
              rel="noreferrer"
            >
              <span>{{ sourceLabel(source) }}</span>
              <small>{{ sourceMeta(source) }}</small>
            </a>
          </div>
        </div>
      </section>

      <section class="map-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Route Map</p>
            <h3 class="panel-title">点位路线图</h3>
          </div>
          <span class="metric-chip">
            <Route :size="15" />
            {{ routedLegCount }} 段路线
          </span>
        </div>
        <AmapTripMap :points="mapPoints" :polylines="routePolylines" />
      </section>

      <section class="weather-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Forecast</p>
            <h3 class="panel-title">天气窗口</h3>
          </div>
          <CloudRain v-if="hasRainyWeather" :size="22" />
          <CloudSun v-else :size="22" />
        </div>

        <div v-if="weatherLoading" class="state-line">正在加载天气信息...</div>
        <div v-else-if="weatherError" class="state-line">{{ weatherError }}</div>
        <div v-else-if="weather" class="weather-grid">
          <div class="weather-meta">
            {{ weatherSourceLabel(weather.source) }}
            <span v-if="weather.report_time">更新：{{ weather.report_time }}</span>
          </div>
          <article
            v-for="day in weather.days"
            :key="`${day.date}-${day.week}`"
            class="weather-card"
          >
            <div class="weather-card__date">{{ formatWeatherDate(day.date, day.week) }}</div>
            <div class="weather-card__temp">
              {{ day.day_temp || "-" }}° / {{ day.night_temp || "-" }}°
            </div>
            <div class="weather-card__desc">白天：{{ day.day_weather || "未知" }}</div>
            <div class="weather-card__desc">夜间：{{ day.night_weather || "未知" }}</div>
            <div class="weather-card__desc" v-if="day.day_wind || day.day_power">
              风力：{{ day.day_wind || "-" }} {{ day.day_power || "" }}
            </div>
          </article>
          <div v-if="weather.advice?.length" class="weather-advice">
            <strong>天气建议</strong>
            <span v-for="item in weather.advice" :key="item">{{ item }}</span>
          </div>
        </div>
        <div v-else class="state-line">暂无天气信息。</div>
      </section>

      <section class="edit-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Tune</p>
            <h3 class="panel-title">多轮调整</h3>
          </div>
          <span class="metric-chip">
            <WandSparkles :size="15" />
            {{ editRevisionCount }} 轮
          </span>
        </div>

        <div class="edit-controls">
          <div v-if="editMessages.length" class="edit-thread">
            <article
              v-for="(item, index) in editMessages"
              :key="`${item.role}-${item.created_at || index}`"
              class="edit-message"
              :class="`edit-message--${item.role}`"
            >
              <div>
                <strong>{{ editRoleLabel(item.role) }}</strong>
                <span>{{ formatEditTime(item.created_at) }}</span>
              </div>
              <p>{{ item.content }}</p>
            </article>
          </div>
          <div v-if="lastChangeSummary.length" class="edit-summary">
            <strong>本轮修改</strong>
            <span v-for="item in lastChangeSummary" :key="item">{{ item }}</span>
          </div>
          <div v-if="editIssues.length" class="edit-issues">
            <strong>仍需留意</strong>
            <span v-for="issue in editIssues" :key="`${issue.code}-${issue.day_index || 0}`">
              {{ issue.day_index ? `D${issue.day_index}：` : "" }}{{ issue.message }}
            </span>
          </div>
          <label class="field-label">
            <PencilLine :size="15" />
            调整范围
          </label>
          <a-select v-model:value="editScope" :options="editScopeOptions" />
          <label class="field-label">
            <Sparkles :size="15" />
            调整说明
          </label>
          <a-textarea v-model:value="editInstruction" :rows="4" />
          <button class="command-button" :disabled="editing" type="button" @click="handleEdit">
            <WandSparkles :size="17" />
            {{ editing ? "调整中" : "应用调整" }}
          </button>
        </div>
      </section>

      <section class="day-budget-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Daily Spend</p>
            <h3 class="panel-title">按天花费</h3>
          </div>
        </div>

        <div class="day-budget-grid">
          <article v-for="item in dayBudgetItems" :key="item.key" class="day-budget-card">
            <div class="day-budget-card__header">
              <strong>{{ item.title }}</strong>
              <span>{{ item.subtitle }}</span>
            </div>
            <div class="day-budget-card__body">
              <div><span>门票</span><strong>{{ money(item.tickets) }}</strong></div>
              <div><span>餐饮</span><strong>{{ money(item.meals) }}</strong></div>
              <div><span>交通</span><strong>{{ money(item.transport) }}</strong></div>
              <div><span>住宿</span><strong>{{ money(item.hotel) }}</strong></div>
              <div class="day-budget-card__total"><span>小计</span><strong>{{ money(item.total) }}</strong></div>
            </div>
          </article>
        </div>
      </section>

      <section class="points-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Places</p>
            <h3 class="panel-title">点位详情</h3>
          </div>
        </div>

        <div class="point-grid">
          <article v-for="point in mapPoints" :key="point.key" class="point-card">
            <div v-if="hasUsableImage(point.imageUrl)" class="point-card__image">
              <img :src="point.imageUrl" :alt="point.name" loading="lazy" @error="markImageFailed(point.imageUrl)" />
            </div>
            <div v-else class="point-card__image point-card__image--empty">
              <MapPinned :size="24" />
            </div>
            <div class="point-card__body">
              <span class="point-card__badge">D{{ point.dayIndex }} / {{ point.kind === "meal" ? "餐饮" : "景点" }}</span>
              <h4>{{ point.name }}</h4>
              <p>{{ point.address }}</p>
              <p>{{ point.description }}</p>
            </div>
          </article>
        </div>
      </section>

      <section class="timeline-panel surface-panel">
        <div class="panel-header">
          <div>
            <p class="panel-kicker">Timeline</p>
            <h3 class="panel-title">每日路线</h3>
          </div>
        </div>

        <div class="timeline-list">
          <details v-for="day in itinerary.days" :key="day.day_index" class="day-card" open>
            <summary>
              <span class="day-card__index">D{{ day.day_index }}</span>
              <span>
                <strong>{{ day.theme || "未命名主题" }}</strong>
                <small>{{ day.date || "日期待定" }} / {{ dayPointCount(day) }} 个安排</small>
              </span>
            </summary>

            <div class="day-card__body">
              <div v-if="day.spots.length" class="day-section">
                <h4>景点</h4>
                <article v-for="spot in day.spots" :key="spot.name">
                  <strong>{{ spot.start_time || "时间待定" }} {{ spot.name }}</strong>
                  <span>{{ spot.description || spot.address || spot.location || "暂无说明" }}</span>
                </article>
              </div>

              <div v-if="day.meals.length" class="day-section">
                <h4>餐饮</h4>
                <article v-for="meal in day.meals" :key="meal.name">
                  <strong>{{ meal.meal_type }} / {{ meal.name }}</strong>
                  <span>{{ meal.notes || meal.address || meal.location || "暂无说明" }}</span>
                </article>
              </div>

              <div v-if="day.transport.length" class="day-section">
                <h4>交通</h4>
                <article v-for="item in day.transport" :key="`${item.mode}-${item.from_place}-${item.to_place}`">
                  <strong>{{ routeModeText(item) }} / {{ joinPlaces(item) }}</strong>
                  <span class="route-meta">{{ routeMeta(item) }}</span>
                  <span v-if="item.route_summary" class="route-summary">{{ item.route_summary }}</span>
                  <span v-if="item.route_warning" class="route-warning">{{ item.route_warning }}</span>
                </article>
              </div>

              <div v-if="day.hotel" class="day-section">
                <h4>住宿</h4>
                <article>
                  <strong>{{ day.hotel.name }}</strong>
                  <span>{{ day.hotel.address || day.hotel.location || day.hotel.level || "住宿信息待补充" }}</span>
                </article>
              </div>

              <div v-if="day.notes.length" class="day-section">
                <h4>备注</h4>
                <article v-for="note in day.notes" :key="note">
                  <span>{{ note }}</span>
                </article>
              </div>
            </div>
          </details>
        </div>
      </section>
    </div>
  </section>

  <section v-else class="empty-state surface-panel">
    <Sparkles :size="32" />
    <h2>还没有生成结果</h2>
    <p>先回到规划舱生成一条 itinerary，路线板就会显示地图、天气、预算和每日安排。</p>
    <button class="command-button" type="button" @click="$emit('backHome')">
      <ArrowLeft :size="17" />
      返回规划舱
    </button>
  </section>
</template>

<style scoped>
.result-dashboard {
  display: grid;
  gap: 16px;
}

.result-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 20px;
  align-items: end;
  padding: 24px;
  border-radius: 8px;
  background:
    linear-gradient(135deg, rgba(15, 118, 110, 0.92), rgba(40, 83, 107, 0.94)),
    var(--ink);
  color: #fffdf7;
  box-shadow: var(--shadow);
}

.result-hero__copy h2 {
  margin: 0;
  font-size: 46px;
  line-height: 1;
  font-weight: 950;
}

.result-hero__copy p:last-child {
  max-width: 840px;
  margin: 14px 0 0;
  color: rgba(255, 253, 247, 0.78);
  line-height: 1.7;
}

.result-hero .panel-kicker {
  color: #fbd38d;
}

.result-hero__stats {
  display: grid;
  gap: 8px;
  min-width: 180px;
}

.result-hero__stats span {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 38px;
  padding: 8px 10px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  font-weight: 850;
}

.action-rail {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 12px;
}

.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}

.overview-panel,
.budget-panel,
.evidence-panel,
.weather-panel,
.edit-panel,
.day-budget-panel,
.points-panel,
.timeline-panel,
.map-panel {
  padding: 18px;
}

.overview-panel {
  grid-column: span 7;
}

.budget-panel {
  grid-column: span 5;
}

.evidence-panel {
  grid-column: 1 / -1;
}

.map-panel {
  grid-column: span 8;
  min-height: 540px;
}

.weather-panel {
  grid-column: span 4;
}

.edit-panel {
  grid-column: span 4;
}

.day-budget-panel {
  grid-column: span 8;
}

.points-panel,
.timeline-panel {
  grid-column: 1 / -1;
}

.overview-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-top: 16px;
}

.overview-metrics article {
  display: grid;
  gap: 5px;
  padding: 14px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}

.overview-metrics strong {
  color: var(--ink);
  font-size: 28px;
}

.overview-metrics span,
.budget-row span,
.state-line,
.weather-card__desc {
  color: var(--muted);
}

.tips-panel {
  margin-top: 14px;
  padding: 14px;
  border-radius: 8px;
  background: rgba(192, 86, 33, 0.08);
  border: 1px solid rgba(192, 86, 33, 0.18);
}

.mini-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--accent-2);
  font-weight: 900;
}

.tips-panel ul {
  display: grid;
  gap: 8px;
  margin: 10px 0 0;
  padding-left: 18px;
  color: var(--muted-strong);
  line-height: 1.65;
}

.budget-list,
.evidence-grid,
.weather-grid,
.edit-controls,
.timeline-list {
  display: grid;
  gap: 10px;
  margin-top: 16px;
}

.budget-row {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  padding: 12px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}

.budget-icon {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: var(--surface);
  color: var(--accent);
  border: 1px solid var(--line);
}

.budget-row strong {
  color: var(--ink);
  font-size: 18px;
}

.evidence-meter {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.evidence-meter article {
  display: grid;
  gap: 4px;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid rgba(15, 118, 110, 0.16);
  background: rgba(15, 118, 110, 0.07);
}

.evidence-meter strong {
  color: var(--ink);
  font-size: 24px;
  font-weight: 950;
}

.evidence-meter span {
  color: var(--muted);
  font-size: 13px;
  font-weight: 800;
}

.evidence-summary {
  display: grid;
  gap: 6px;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--surface-2);
  color: var(--muted-strong);
  line-height: 1.55;
}

.evidence-warning {
  color: #c05621;
  font-weight: 850;
}

.evidence-verification {
  color: var(--accent);
  font-weight: 850;
}

.evidence-claims {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 10px;
}

.evidence-claim {
  display: grid;
  gap: 7px;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid rgba(15, 118, 110, 0.18);
  background: var(--surface);
}

.evidence-claim--review {
  border-color: rgba(192, 86, 33, 0.24);
  background: rgba(192, 86, 33, 0.07);
}

.evidence-claim__header {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}

.evidence-chip-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-start;
  gap: 6px;
  align-items: center;
}

.evidence-chip {
  display: inline-flex;
  width: fit-content;
  min-height: 24px;
  align-items: center;
  padding: 3px 8px;
  border-radius: 8px;
  border: 1px solid rgba(15, 118, 110, 0.18);
  background: rgba(15, 118, 110, 0.07);
  color: var(--accent);
  font-size: 12px;
  font-weight: 900;
}

.evidence-chip--strong {
  border-color: rgba(15, 118, 110, 0.34);
  background: rgba(15, 118, 110, 0.14);
  color: var(--ink);
}

.evidence-claim strong {
  color: var(--ink);
}

.evidence-claim span,
.evidence-claim small {
  color: var(--muted);
  line-height: 1.45;
}

.evidence-claim p {
  margin: 0;
  color: var(--muted-strong);
  line-height: 1.55;
}

.evidence-claim a {
  width: fit-content;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--accent);
  font-weight: 900;
  text-decoration: none;
}

.evidence-sources {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
}

.evidence-sources a {
  display: grid;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--surface-2);
  text-decoration: none;
}

.evidence-sources span {
  color: var(--ink);
  font-weight: 900;
  overflow-wrap: anywhere;
}

.evidence-sources small {
  color: var(--muted);
  line-height: 1.45;
}

.evidence-source--disabled {
  pointer-events: none;
}

.map-panel :deep(.trip-map) {
  margin-top: 16px;
  height: 448px;
}

.weather-card {
  padding: 12px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}

.weather-meta,
.weather-advice {
  display: grid;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(15, 118, 110, 0.08);
  border: 1px solid rgba(15, 118, 110, 0.16);
  color: var(--accent);
  font-weight: 900;
}

.weather-meta span,
.weather-advice span {
  color: var(--muted);
  font-weight: 650;
  line-height: 1.55;
}

.weather-card__date {
  color: var(--ink);
  font-weight: 900;
}

.weather-card__temp {
  margin: 8px 0;
  color: var(--accent);
  font-size: 22px;
  font-weight: 950;
}

.state-line {
  margin-top: 16px;
  line-height: 1.7;
}

.edit-controls .command-button {
  width: 100%;
  margin-top: 2px;
}

.edit-thread {
  display: grid;
  gap: 8px;
  max-height: 260px;
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface-2);
}

.edit-message {
  display: grid;
  gap: 6px;
  padding: 10px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--surface);
}

.edit-message--user {
  border-color: rgba(15, 118, 110, 0.24);
  background: rgba(15, 118, 110, 0.07);
}

.edit-message--assistant {
  border-color: rgba(192, 86, 33, 0.22);
  background: rgba(192, 86, 33, 0.07);
}

.edit-message div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.edit-message strong {
  color: var(--ink);
}

.edit-message span {
  color: var(--muted);
  font-size: 12px;
}

.edit-message p {
  margin: 0;
  color: var(--muted-strong);
  line-height: 1.55;
  white-space: pre-line;
}

.edit-summary,
.edit-issues {
  display: grid;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
}

.edit-summary {
  border: 1px solid rgba(15, 118, 110, 0.18);
  background: rgba(15, 118, 110, 0.08);
}

.edit-issues {
  border: 1px solid rgba(192, 86, 33, 0.2);
  background: rgba(192, 86, 33, 0.08);
}

.edit-summary strong,
.edit-issues strong {
  color: var(--ink);
}

.edit-summary span,
.edit-issues span {
  color: var(--muted-strong);
  line-height: 1.55;
}

.day-budget-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.day-budget-card {
  overflow: hidden;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--surface);
}

.day-budget-card__header {
  display: grid;
  gap: 4px;
  padding: 13px;
  background: var(--ink);
  color: #fffdf7;
}

.day-budget-card__header span {
  color: rgba(255, 253, 247, 0.66);
  font-size: 13px;
}

.day-budget-card__body {
  display: grid;
  gap: 8px;
  padding: 13px;
}

.day-budget-card__body div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: var(--muted);
}

.day-budget-card__body strong {
  color: var(--ink);
}

.day-budget-card__total {
  padding-top: 8px;
  border-top: 1px solid var(--line);
}

.point-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.point-card {
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface);
}

.point-card__image {
  height: 138px;
  overflow: hidden;
  background-color: var(--surface-3);
}

.point-card__image img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.point-card__image--empty {
  display: grid;
  place-items: center;
  color: var(--muted);
}

.point-card__body {
  display: grid;
  gap: 8px;
  padding: 13px;
}

.point-card__badge {
  width: fit-content;
  padding: 5px 8px;
  border-radius: 8px;
  background: rgba(15, 118, 110, 0.1);
  color: var(--accent);
  font-size: 12px;
  font-weight: 900;
}

.point-card h4 {
  margin: 0;
  color: var(--ink);
  font-size: 17px;
}

.point-card p {
  margin: 0;
  color: var(--muted);
  line-height: 1.55;
}

.day-card {
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface);
  overflow: hidden;
}

.day-card summary {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  cursor: pointer;
  list-style: none;
}

.day-card summary::-webkit-details-marker {
  display: none;
}

.day-card__index {
  width: 44px;
  height: 44px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: var(--accent);
  color: #fffdf7;
  font-weight: 950;
}

.day-card summary strong,
.day-card summary small {
  display: block;
}

.day-card summary strong {
  color: var(--ink);
  font-size: 17px;
}

.day-card summary small {
  margin-top: 3px;
  color: var(--muted);
}

.day-card__body {
  display: grid;
  gap: 14px;
  padding: 0 14px 14px 70px;
}

.day-section {
  display: grid;
  gap: 8px;
}

.day-section h4 {
  margin: 0;
  color: var(--accent-2);
  font-size: 13px;
  font-weight: 950;
}

.day-section article {
  display: grid;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}

.day-section strong {
  color: var(--ink);
}

.day-section span {
  color: var(--muted);
  line-height: 1.55;
}

.day-section .route-meta {
  color: var(--accent);
  font-weight: 850;
}

.day-section .route-summary {
  color: var(--muted-strong);
}

.day-section .route-warning {
  color: #c05621;
  font-weight: 850;
}

.empty-state {
  min-height: 360px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 12px;
  padding: 32px;
  text-align: center;
}

.empty-state h2,
.empty-state p {
  margin: 0;
}

.empty-state p {
  max-width: 560px;
  color: var(--muted);
  line-height: 1.7;
}

@media (max-width: 1180px) {
  .overview-panel,
  .budget-panel,
  .evidence-panel,
  .map-panel,
  .weather-panel,
  .edit-panel,
  .day-budget-panel {
    grid-column: 1 / -1;
  }
}

@media (max-width: 760px) {
  .result-hero {
    grid-template-columns: 1fr;
    padding: 18px;
  }

  .result-hero__copy h2 {
    font-size: 34px;
  }

  .overview-metrics {
    grid-template-columns: 1fr;
  }

  .evidence-meter {
    grid-template-columns: 1fr;
  }

  .panel-header {
    align-items: start;
    flex-direction: column;
  }

  .day-card__body {
    padding-left: 14px;
  }
}
</style>
