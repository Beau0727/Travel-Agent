# Go 后端开发说明

`backend-go/` 是旅行助手 Agent 当前使用的后端实现。它提供行程生成、编辑、保存、历史、天气示例、地图补全和 Markdown 导出接口，并通过固定步骤 Agent 或可选 tool-calling Agent 组织生成流程。

没有 `LLM_API_KEY` 时，后端会使用规则规划器，仍能跑通生成、保存、历史和 Markdown 导出。配置 OpenAI-compatible 模型后，生成流程会进入 tool-calling Agent，让模型选择 RAG、在线攻略研究、天气、地图补全、校验、修复等工具。

## 目录结构

```text
backend-go/
├── cmd/server/main.go              # 程序入口
├── data/                           # 本地 Markdown 攻略数据
├── internal/bootstrap/             # 应用装配和依赖注入
├── internal/application/           # 应用用例层
├── internal/config/config.go       # 环境变量读取
├── internal/domain/models.go       # 请求和响应模型
├── internal/agent/                 # Agent 状态、步骤、tool-calling 循环
├── internal/transport/http/        # HTTP 路由和 handler
├── internal/infrastructure/        # 外部资源适配器入口
├── internal/rag/                   # Markdown 攻略关键词检索
├── internal/services/              # 行程、天气、导出、Web research 等服务
├── internal/storage/               # TripRepository 接口和 JSON 文件实现
├── internal/tools/                 # Agent 工具封装
└── internal/validators/            # 预算、节奏、偏好校验
```

## 核心接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查 |
| `POST` | `/trip/generate` | 生成行程 |
| `POST` | `/trip/edit` | 编辑行程 |
| `POST` | `/trip/save` | 保存行程 |
| `GET` | `/trip` | 查看历史列表 |
| `GET` | `/trip/{trip_id}` | 查看详情 |
| `DELETE` | `/trip/{trip_id}` | 删除行程 |
| `GET` | `/weather/forecast?city=大理` | 获取 demo 天气数据 |
| `GET` | `/export/{trip_id}/markdown` | 导出 Markdown |

当前后端没有实现 PDF 导出。

## 启动方式

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/server
```

默认端口是 `8000`。启动后访问：

```text
http://127.0.0.1:8000/health
```

## 配置

后端读取 `backend-go/.env`，也可以直接用环境变量覆盖。参考 `backend-go/.env.example`：

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

说明：

- `DATA_DIR` 默认指向 `backend-go/data`。
- `STORAGE_FILE` 是 JSON 行程存储文件。
- `LLM_API_KEY` 为空时使用规则规划器。
- `ENABLE_AMAP_ENRICHMENT=true` 且配置 `AMAP_API_KEY` 后，会尝试补充地址、经纬度、POI ID 和图片。
- `ENABLE_WEB_RESEARCH=true` 时，需要配置返回 JSON 的搜索服务地址 `WEB_SEARCH_ENDPOINT`。

## 测试

```powershell
cd F:\Code\Travel-Agent\backend-go
go test ./...
```

## 当前限制

- PDF 导出未实现，只支持 Markdown。
- 天气服务当前返回 demo 数据。
- RAG 当前是 Markdown 关键词检索，不是向量检索。
- JSON 文件存储适合 demo，不适合生产级并发写入。
- 地图补全依赖高德 Web 服务 Key；没有 Key 时会跳过，不影响行程生成。
