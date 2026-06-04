# 旅行助手 Agent

旅行助手 Agent 是一个面向中文旅行场景的 AI 行程规划原型。当前主实现由 Go 后端和 Vue 前端组成：用户填写目的地、日期、预算、人数和偏好后，系统生成结构化 itinerary，并支持地图点位展示、天气提示、智能编辑、历史保存和 Markdown 导出。

没有模型密钥时，后端会使用规则规划器兜底；配置 OpenAI-compatible 模型后，可以启用 tool-calling Agent，让模型选择 RAG、在线攻略研究、天气、地图补全、校验和修复等工具。

## 技术栈

- 后端：Go 1.22，标准库 HTTP 服务
- 前端：Vue 3，TypeScript，Vite，Ant Design Vue，Axios
- Agent：固定步骤 Agent + 可选 OpenAI-compatible tool-calling Agent
- RAG：本地 Markdown 关键词检索
- 存储：JSON 文件
- 地图：高德 Web 服务坐标补全 + 高德 JavaScript API 前端地图
- 天气：当前返回 demo 天气数据

## 目录结构

```text
Travel-Agent/
├── backend-go/                 # Go 后端
│   ├── cmd/server/main.go       # 服务入口
│   ├── data/                    # 本地 Markdown 攻略数据
│   ├── internal/                # 后端业务代码
│   ├── go.mod
│   └── README.md
├── frontend/                   # Vue 前端
│   ├── src/
│   ├── package.json
│   └── README.md
├── assets/showcase/             # 项目截图
└── README.md
```

## 后端接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查 |
| `POST` | `/trip/generate` | 生成行程 |
| `POST` | `/trip/edit` | 根据自然语言编辑行程 |
| `POST` | `/trip/save` | 保存行程 |
| `GET` | `/trip` | 查看历史行程列表 |
| `GET` | `/trip/{trip_id}` | 查看行程详情 |
| `DELETE` | `/trip/{trip_id}` | 删除行程 |
| `GET` | `/weather/forecast?city=大理` | 获取 demo 天气数据 |
| `GET` | `/export/{trip_id}/markdown` | 导出 Markdown |

当前后端没有实现 PDF 导出，前端也只保留 Markdown 导出入口。

## 快速启动

启动后端：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/server
```

默认端口是 `8000`。启动后访问：

```text
http://127.0.0.1:8000/health
```

启动前端：

```powershell
cd F:\Code\Travel-Agent\frontend
npm install
npm run dev
```

浏览器访问：

```text
http://127.0.0.1:5173
```

## 配置

后端读取 `backend-go/.env` 或环境变量。可以参考 `backend-go/.env.example`：

```env
PORT=8000
DATA_DIR=data
STORAGE_FILE=data/trips.json

LLM_API_KEY=
LLM_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
LLM_MODEL=qwen-max
LLM_TIMEOUT_SECONDS=60

ENABLE_AMAP_ENRICHMENT=false
AMAP_API_KEY=
AMAP_BASE_URL=https://restapi.amap.com/v3

ENABLE_WEB_RESEARCH=false
WEB_SEARCH_ENDPOINT=
WEB_SEARCH_API_KEY=
WEB_RESEARCH_TIMEOUT_SECONDS=20
WEB_RESEARCH_MAX_PAGES=3
```

前端读取 `frontend/.env`，可以参考 `frontend/.env.example`：

```env
VITE_API_BASE_URL=http://127.0.0.1:8000
VITE_AMAP_JS_KEY=你的高德 JavaScript API Key
```

## 验证

后端测试：

```powershell
cd F:\Code\Travel-Agent\backend-go
go test ./...
```

前端构建：

```powershell
cd F:\Code\Travel-Agent\frontend
npm run build
```

Vite 可能提示主 chunk 大于 500 kB，这是依赖体积警告，不影响运行。

## 当前限制

- PDF 导出未实现，只支持 Markdown。
- 天气服务当前是 demo 数据。
- RAG 当前是 Markdown 关键词检索，不是向量检索。
- JSON 文件存储适合 demo，不适合生产级并发写入。
- 地图补全依赖高德 Web 服务 Key；没有 Key 时会跳过，不影响行程生成。
