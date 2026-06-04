# Frontend 开发说明

`frontend/` 是旅行助手 Agent 的 Vue 前端，使用 Vue 3、TypeScript、Vite、Ant Design Vue 和 Axios。它和 `backend-go/` 联调，提供规划、结果展示、保存、历史、地图、天气、智能调整和 Markdown 导出。

## 当前能力

- 规划页：填写目的地、日期、人数、预算、偏好和备注，调用 `/trip/generate`
- 结果页：展示行程概览、预算明细、按天花费、地图、天气、点位图片和每日安排
- 保存：调用 `/trip/save`
- 历史列表：调用 `/trip` 和 `/trip/{trip_id}`
- 删除历史行程：调用 `DELETE /trip/{trip_id}`
- 智能调整：调用 `/trip/edit`
- 导出：支持 Markdown，导出前会先保存当前页面数据
- 地图：接入高德 JavaScript API
- 天气：展示 Go 后端 `/weather/forecast` 返回的数据

当前 Go 后端尚未实现 PDF 导出，所以前端不提供 PDF 导出入口。

## 环境变量

在 `frontend/` 目录下创建 `.env`：

```env
VITE_API_BASE_URL=http://127.0.0.1:8000
VITE_AMAP_JS_KEY=你的高德 JavaScript API key
```

说明：

- `VITE_API_BASE_URL` 必须是浏览器能访问到的 Go 后端地址。
- 高德前端地图需要 JavaScript API key，不是后端 Web 服务 key。
- 修改 `.env` 后需要重启 `npm run dev`。

## 启动方式

先启动 Go 后端：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/server
```

确认后端可访问：

```text
http://127.0.0.1:8000/health
```

再启动前端：

```powershell
cd F:\Code\Travel-Agent\frontend
npm install
npm run dev
```

浏览器访问：

```text
http://127.0.0.1:5173
```

## 构建验证

```powershell
cd F:\Code\Travel-Agent\frontend
npm run build
```

Vite 可能提示主 chunk 大于 500 kB，这是依赖体积警告，不影响运行。
