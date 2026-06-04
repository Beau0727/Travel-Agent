<script setup lang="ts">
import { computed, ref } from "vue";
import { Archive, Compass, MapPinned, Route, Sparkles } from "@lucide/vue";

import type { Itinerary } from "./types";
import History from "./views/History.vue";
import Home from "./views/Home.vue";
import Result from "./views/Result.vue";

const currentView = ref<"home" | "result" | "history">("home");
const latestItinerary = ref<Itinerary | null>(null);

const navItems = computed(() => [
  {
    key: "home" as const,
    label: "规划舱",
    description: "输入偏好",
    icon: Compass,
    disabled: false,
  },
  {
    key: "result" as const,
    label: "路线板",
    description: latestItinerary.value ? latestItinerary.value.destination : "等待生成",
    icon: Route,
    disabled: !latestItinerary.value,
  },
  {
    key: "history" as const,
    label: "档案库",
    description: "已保存行程",
    icon: Archive,
    disabled: false,
  },
]);

const workspaceTitle = computed(() => {
  if (currentView.value === "history") {
    return "历史行程档案";
  }

  if (currentView.value === "result" && latestItinerary.value) {
    return `${latestItinerary.value.destination} 行程总览`;
  }

  return "创建一条新的旅行路线";
});

function handleGenerated(itinerary: Itinerary) {
  latestItinerary.value = itinerary;
  currentView.value = "result";
}

function openTrip(itinerary: Itinerary) {
  latestItinerary.value = itinerary;
  currentView.value = "result";
}

function updateCurrentItinerary(itinerary: Itinerary) {
  latestItinerary.value = itinerary;
  currentView.value = "result";
}

function navigate(view: "home" | "result" | "history") {
  if (view === "result" && !latestItinerary.value) {
    return;
  }

  currentView.value = view;
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <button class="brand-button" type="button" @click="navigate('home')">
        <span class="brand-mark">
          <MapPinned :size="22" stroke-width="2.4" />
        </span>
        <span>
          <span class="brand-name">智旅云图</span>
          <span class="brand-subtitle">AI Travel Console</span>
        </span>
      </button>

      <nav class="workspace-nav" aria-label="主导航">
        <button
          v-for="item in navItems"
          :key="item.key"
          class="workspace-nav__item"
          :class="{ 'workspace-nav__item--active': currentView === item.key }"
          :disabled="item.disabled"
          type="button"
          @click="navigate(item.key)"
        >
          <component :is="item.icon" :size="18" />
          <span>
            <span class="workspace-nav__label">{{ item.label }}</span>
            <span class="workspace-nav__description">{{ item.description }}</span>
          </span>
        </button>
      </nav>

      <div class="topbar-status">
        <Sparkles :size="17" />
        <span>{{ latestItinerary ? "已有可查看行程" : "准备生成路线" }}</span>
      </div>
    </header>

    <main class="workspace">
      <div class="workspace-heading">
        <div>
          <p class="workspace-kicker">Workspace</p>
          <h1>{{ workspaceTitle }}</h1>
        </div>
        <div class="workspace-meta">
          <span>{{ currentView === "home" ? "Step 01" : currentView === "result" ? "Step 02" : "Archive" }}</span>
          <strong>{{ latestItinerary?.days.length || 0 }} 天</strong>
        </div>
      </div>

      <Home v-if="currentView === 'home'" @generated="handleGenerated" />
      <Result
        v-else-if="currentView === 'result'"
        :itinerary="latestItinerary"
        @back-home="navigate('home')"
        @view-history="navigate('history')"
        @updated="updateCurrentItinerary"
      />
      <History
        v-else
        :active="currentView === 'history'"
        @open-trip="openTrip"
      />
    </main>
  </div>
</template>
