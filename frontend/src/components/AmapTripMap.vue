<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { MapPinned } from "@lucide/vue";

interface TripMapPoint {
  key: string;
  kind?: "spot" | "meal";
  dayIndex: number;
  date: string;
  theme: string;
  name: string;
  address: string;
  latitude: number | null | undefined;
  longitude: number | null | undefined;
  poiId: string | null | undefined;
  imageUrl?: string | null;
  description: string;
}

const props = defineProps<{
  points: TripMapPoint[];
  polylines?: string[];
}>();

declare global {
  interface Window {
    AMap?: any;
  }
}

const mapContainer = ref<HTMLDivElement | null>(null);
const mapInstance = ref<any>(null);
const markerList = ref<any[]>([]);
const routeLines = ref<any[]>([]);
const loadError = ref("");

const amapKey = import.meta.env.VITE_AMAP_JS_KEY;

const validPoints = computed(() =>
  props.points.filter(
    (point) => point.longitude != null && point.latitude != null
  )
);

const routePolylinePaths = computed(() =>
  (props.polylines ?? [])
    .map((polyline) => parseAmapPolyline(polyline))
    .filter((path) => path.length >= 2)
);

function parseAmapPolyline(polyline: string): [number, number][] {
  return polyline
    .split(/[;|]/)
    .map((pair) => pair.trim())
    .filter(Boolean)
    .map((pair) => {
      const [lngText, latText] = pair.split(",");
      const lng = Number(lngText);
      const lat = Number(latText);
      if (!Number.isFinite(lng) || !Number.isFinite(lat)) {
        return null;
      }
      return [lng, lat] as [number, number];
    })
    .filter((point): point is [number, number] => point !== null);
}

function escapeHtml(value?: string | null): string {
  return String(value ?? "").replace(/[&<>"']/g, (char) => {
    const map: Record<string, string> = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return map[char] || char;
  });
}

function fallbackImageHtml() {
  return `
    <div style="
      position:absolute;
      inset:0;
      display:grid;
      place-items:center;
      background:#ebe4d5;
      color:#53616c;
      font-size:12px;
      font-weight:800;
    ">暂无图片</div>
  `;
}

function bubbleImageHtml(point: TripMapPoint) {
  const fallback = fallbackImageHtml();
  if (!point.imageUrl) {
    return `<div style="position:relative;width:100%;height:100%;overflow:hidden;">${fallback}</div>`;
  }

  return `
    <div style="position:relative;width:100%;height:100%;overflow:hidden;">
      ${fallback}
      <img
        src="${escapeHtml(point.imageUrl)}"
        alt="${escapeHtml(point.name)}"
        style="position:absolute;inset:0;width:100%;height:100%;object-fit:cover;"
        onerror="this.style.display='none'"
      />
    </div>
  `;
}

function clearOverlays() {
  if (!mapInstance.value) {
    return;
  }

  markerList.value.forEach((marker) => {
    mapInstance.value.remove(marker);
  });
  markerList.value = [];

  routeLines.value.forEach((line) => {
    mapInstance.value.remove(line);
  });
  routeLines.value = [];
}

function renderMarkers() {
  if (!window.AMap || !mapInstance.value) {
    return;
  }

  clearOverlays();

  const sorted = [...validPoints.value].sort((a, b) => a.dayIndex - b.dayIndex);
  const bounds: [number, number][] = [];
  const routePath: [number, number][] = [];

  sorted.forEach((point) => {
    const position: [number, number] = [point.longitude as number, point.latitude as number];
    bounds.push(position);
    routePath.push(position);

    const markerText = point.kind === "meal" ? "餐" : "游";
    const marker = new window.AMap.Marker({
      position,
      title: point.name,
      offset: new window.AMap.Pixel(-17, -40),
      content: `
        <div style="
          display:grid;
          place-items:center;
          width:34px;
          height:40px;
          border-radius:10px 10px 10px 2px;
          transform:rotate(45deg);
          background:${point.kind === "meal" ? "#c05621" : "#0f766e"};
          color:#fffdf7;
          box-shadow:0 10px 20px rgba(31,41,51,0.22);
          border:2px solid #fffdf7;
        ">
          <span style="
            transform:rotate(-45deg);
            font-size:12px;
            font-weight:900;
            line-height:1;
          ">${markerText}</span>
        </div>
      `,
    });

    const imageHtml = bubbleImageHtml(point);
    const safeName = escapeHtml(point.name);
    const safeAddress = escapeHtml(point.address);
    const safeTheme = escapeHtml(point.theme);

    const bubble = new window.AMap.Marker({
      position,
      offset: new window.AMap.Pixel(18, -58),
      content: `
        <div style="
          width:132px;
          overflow:hidden;
          border-radius:8px;
          background:#fffdf7;
          border:1px solid rgba(37,49,59,0.16);
          box-shadow:0 14px 26px rgba(31,41,51,0.18);
          font-family:'Microsoft YaHei','PingFang SC','Segoe UI',sans-serif;
        ">
          <div style="width:132px;height:76px;overflow:hidden;">${imageHtml}</div>
          <div style="padding:7px 9px;">
            <div style="display:flex;align-items:center;justify-content:space-between;gap:6px;margin-bottom:4px;">
              <span style="color:#c05621;font-size:11px;font-weight:900;">D${point.dayIndex}</span>
              <span style="color:#71808c;font-size:11px;">${point.kind === "meal" ? "餐饮" : "景点"}</span>
            </div>
            <div style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#1f2933;font-size:12px;font-weight:900;">${safeName}</div>
          </div>
        </div>
      `,
      zIndex: 100,
    });

    const infoWindow = new window.AMap.InfoWindow({
      offset: new window.AMap.Pixel(0, -38),
      content: `
        <div style="max-width:260px;padding:6px 3px;line-height:1.7;color:#24313b;font-family:'Microsoft YaHei','PingFang SC','Segoe UI',sans-serif;">
          <strong style="color:#1f2933;">${safeName}</strong><br/>
          <span>第 ${point.dayIndex} 天 · ${safeTheme}</span><br/>
          <span>${safeAddress}</span>
        </div>
      `,
    });

    marker.on("click", () => {
      infoWindow.open(mapInstance.value, position);
    });

    mapInstance.value.add(marker);
    mapInstance.value.add(bubble);
    markerList.value.push(marker);
    markerList.value.push(bubble);
  });

  const actualRoutePaths = routePolylinePaths.value;
  const pathsToDraw = actualRoutePaths.length ? actualRoutePaths : [routePath];
  pathsToDraw
    .filter((path) => path.length >= 2)
    .forEach((path, index) => {
      const line = new window.AMap.Polyline({
        path,
        strokeColor: actualRoutePaths.length ? "#2563eb" : "#0f766e",
        strokeWeight: 4,
        strokeOpacity: 0.86,
        strokeStyle: "solid",
        lineJoin: "round",
        lineCap: "round",
        showDir: true,
        dirColor: "#c05621",
        dirSize: 9,
        borderWeight: 2,
        borderColor: "rgba(255,253,247,0.92)",
        zIndex: 50 - index,
      });
      mapInstance.value.add(line);
      routeLines.value.push(line);
    });

  if (bounds.length === 1) {
    mapInstance.value.setZoomAndCenter(13, bounds[0]);
  } else if (bounds.length > 1) {
    mapInstance.value.setFitView([...markerList.value, ...routeLines.value], false, [70, 70, 70, 70]);
  }
}

function ensureMapScript(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (window.AMap) {
      resolve();
      return;
    }

    const existingScript = document.querySelector<HTMLScriptElement>(
      'script[data-amap-loader="true"]'
    );

    if (existingScript) {
      existingScript.addEventListener("load", () => resolve(), { once: true });
      existingScript.addEventListener("error", () => reject(new Error("高德地图脚本加载失败。")), {
        once: true,
      });
      return;
    }

    const script = document.createElement("script");
    script.src = `https://webapi.amap.com/maps?v=2.0&key=${amapKey}`;
    script.async = true;
    script.defer = true;
    script.dataset.amapLoader = "true";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("高德地图脚本加载失败。"));
    document.head.appendChild(script);
  });
}

async function initMap() {
  if (!amapKey) {
    loadError.value = "未配置前端高德 JavaScript Key。";
    return;
  }

  if (!mapContainer.value) {
    return;
  }

  try {
    loadError.value = "";
    await ensureMapScript();

    if (!window.AMap) {
      loadError.value = "高德地图对象初始化失败。";
      return;
    }

    mapInstance.value = new window.AMap.Map(mapContainer.value, {
      zoom: 11,
      resizeEnable: true,
      viewMode: "2D",
      mapStyle: "amap://styles/macaron",
    });

    renderMarkers();
  } catch (error) {
    console.error(error);
    loadError.value = "地图加载失败，请检查前端高德 Key 或网络环境。";
  }
}

onMounted(() => {
  void initMap();
});

watch([validPoints, routePolylinePaths], () => {
  if (mapInstance.value) {
    renderMarkers();
  }
});

onBeforeUnmount(() => {
  clearOverlays();
  if (mapInstance.value) {
    mapInstance.value.destroy();
    mapInstance.value = null;
  }
});
</script>

<template>
  <div class="trip-map">
    <div v-if="loadError" class="trip-map__placeholder">
      <MapPinned :size="34" />
      <strong>地图暂未启用</strong>
      <span>{{ loadError }}</span>
    </div>
    <div v-else-if="validPoints.length === 0" class="trip-map__placeholder">
      <MapPinned :size="34" />
      <strong>暂无可展示点位</strong>
      <span>当前 itinerary 里还没有可用的经纬度数据。</span>
    </div>
    <div v-else ref="mapContainer" class="trip-map__canvas"></div>
  </div>
</template>

<style scoped>
.trip-map {
  height: 100%;
  min-height: 320px;
}

.trip-map__canvas,
.trip-map__placeholder {
  width: 100%;
  height: 100%;
  min-height: 320px;
  border-radius: 8px;
}

.trip-map__canvas {
  overflow: hidden;
  border: 1px solid var(--line);
}

.trip-map__placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  border: 1px dashed var(--line-strong);
  background:
    linear-gradient(135deg, rgba(15, 118, 110, 0.08), rgba(192, 86, 33, 0.08)),
    var(--surface-2);
  color: var(--muted);
  text-align: center;
}

.trip-map__placeholder strong {
  color: var(--ink);
  font-size: 22px;
}

.trip-map__placeholder span {
  max-width: 360px;
  color: var(--muted);
  line-height: 1.7;
}
</style>
