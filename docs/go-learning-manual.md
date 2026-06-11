# Go 后端学习手册

这份手册用于快速掌握当前 `backend-go/` 项目。它会把功能、代码目录、Go 语言设计方式放在一起讲，重点覆盖最近新增的高级 RAG、RAG 评测、RAG 对比、HTTP 重排服务和更多攻略语料。

## 1. 先理解项目在做什么

旅行助手后端负责把用户输入的旅行需求变成结构化行程。完整链路是：

```text
Vue 前端
 -> Go HTTP Router
 -> Application Usecase
 -> TravelPlanningAgent
 -> RAG / Web Research / Planner / Weather / Map / Route Tools
 -> ItineraryAssembler
 -> Validator
 -> JSON Repository
```

对应代码入口：

| 职责 | 代码位置 | 学习重点 |
| --- | --- | --- |
| HTTP 服务入口 | `backend-go/cmd/server/main.go` | `main` 函数、配置加载、启动服务 |
| 依赖装配 | `backend-go/internal/bootstrap/app.go` | 依赖注入、按配置选择实现 |
| HTTP 路由 | `backend-go/internal/transport/http/router.go` | 标准库 `net/http`、JSON 编解码 |
| 应用用例 | `backend-go/internal/application` | 将 HTTP 动作转成业务动作 |
| 领域模型 | `backend-go/internal/domain/models.go` | `struct`、JSON tag、嵌套数据结构 |
| Agent 编排 | `backend-go/internal/agent` | 固定步骤 Agent、tool-calling Agent |
| 工具层 | `backend-go/internal/tools` | 把服务能力包装成 Agent 可调用工具 |
| 服务层 | `backend-go/internal/services` | 规划、天气、路线、编辑、导出 |
| 外部适配 | `backend-go/internal/infrastructure` | Qdrant、Ollama、高德、LLM、JSON 文件 |

## 2. 当前技术栈

后端：

- Go 1.22。
- 标准库 HTTP 服务：`net/http`。
- JSON 编解码：`encoding/json`。
- 结构化日志：`log/slog`。
- 配置读取：`.env` + 环境变量。
- 本地存储：JSON 文件仓储。

AI 与检索：

- OpenAI-compatible Chat API。
- 可选 LLM tool-calling Agent。
- 本地 Markdown 词法检索。
- Ollama embedding。
- Qdrant dense 向量检索。
- 新增：Hybrid RAG，融合 dense + lexical。
- 新增：HTTP reranker sidecar。
- 新增：RAG 自动评测与多配置对比。

外部能力：

- 高德 Web 服务：地图点位补全、天气、路线。
- 可选在线搜索与网页摘要。

前端：

- Vue 3、TypeScript、Vite、Ant Design Vue、Axios、高德 JavaScript API。

## 3. Go 项目分层怎么理解

这个项目采用轻量“整洁架构 / 端口与适配器”结构。

核心原则是：

```text
上层业务依赖接口，不依赖具体外部系统。
具体外部系统放到 infrastructure。
bootstrap 负责把接口和实现接起来。
```

例如 RAG：

```text
internal/rag/retriever.go
  定义 Retriever / DetailedRetriever / Reranker 接口

internal/infrastructure/rag/markdown_retriever.go
  实现本地 Markdown 检索

internal/infrastructure/rag/qdrant_retriever.go
  实现 Qdrant 向量检索

internal/infrastructure/rag/hybrid_retriever.go
  实现 dense + lexical + RRF + reranker

internal/bootstrap/app.go
  根据 RAG_BACKEND 选择具体实现
```

这样 Agent 层只知道“我要检索上下文”，不需要知道底层是 Markdown、Qdrant 还是混合检索。

## 4. Go 语言核心设计点

### 4.1 struct 表达数据

核心业务数据在 `internal/domain/models.go`：

```go
type TripRequest struct {
    Destination string   `json:"destination"`
    Preferences []string `json:"preferences"`
}
```

学习重点：

- Go 用 `struct` 表达结构化数据。
- 字段首字母大写表示可被包外访问。
- `json:"destination"` 是 struct tag，用于 HTTP JSON 字段映射。
- `[]string` 是切片，用来表达列表。
- `*float64`、`*HotelItem` 表示字段可为空。

### 4.2 interface 表达能力

仓储、RAG、LLM 都用接口表达能力：

```go
type Retriever interface {
    Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error)
}
```

学习重点：

- Go 的接口是隐式实现的。
- 只要某个类型拥有同名方法，就自动满足接口。
- 上层依赖接口后，可以轻松替换实现。

### 4.3 method 表达对象行为

例如 `TripUsecase`：

```go
func (u *TripUsecase) Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error)
```

学习重点：

- `(u *TripUsecase)` 是方法接收者。
- 指针接收者避免复制对象，并允许修改对象状态。
- Go 常用 `(result, error)` 返回错误，不使用 `try/catch`。

### 4.4 context 贯穿调用链

HTTP、LLM、高德、Qdrant、reranker 都应带 `context.Context`。

学习重点：

- 用于取消请求、设置超时、传递 request id。
- 外部 HTTP 请求应使用 `http.NewRequestWithContext`。
- 这个项目中部分 RAG 适配器为了兼容旧接口内部创建 context，后续可以继续演进成显式传入 `ctx`。

## 5. 请求生成行程的主链路

以 `POST /trip/generate` 为例：

```text
router.handleGenerateTrip
 -> TripUsecase.Generate
 -> ToolCallingTravelPlanningAgent.Generate
 -> runToolLoop
 -> rag_search / web_research / weather_forecast / generate_itinerary_draft
 -> enrich_map / enrich_routes / validate_itinerary / repair_itinerary
 -> finish_itinerary
```

关键代码：

- `internal/transport/http/router.go`：解析 HTTP 请求。
- `internal/application/trip_usecase.go`：应用用例。
- `internal/agent/tool_calling_agent.go`：tool-calling 循环。
- `internal/agent/tool_models.go`：工具入参、出参和结果模型。
- `internal/tools`：具体工具包装。
- `internal/services/itinerary_assembler.go`：把 planner draft 组装为 itinerary。
- `internal/validators`：预算、节奏、偏好、路线校验。

如果没有 `LLM_API_KEY`，系统会回落到固定步骤 Agent 和规则规划器，便于本地学习和调试。

## 6. 新增功能一：高级 RAG

### 6.1 新增了哪些检索后端

现在 `RAG_BACKEND` 支持：

```env
RAG_BACKEND=markdown
RAG_BACKEND=qdrant
RAG_BACKEND=hybrid
```

含义：

- `markdown`：读取 `data/guides/*.md`，用本地词法检索召回。
- `qdrant`：用 Ollama embedding 生成向量，再查 Qdrant。
- `hybrid`：同时跑 Qdrant dense 检索和 Markdown lexical 检索，再用 RRF 融合，可选 reranker 重排。

装配位置在 `internal/bootstrap/app.go`：

```text
if RAG_BACKEND == hybrid -> NewHybridRetriever
if RAG_BACKEND == qdrant -> NewQdrantRetriever
else -> NewMarkdownRetriever
```

### 6.2 新增的 RAG 接口

`internal/rag/retriever.go` 现在有三类接口：

```go
type Retriever interface {
    Retrieve(...) ([]string, error)
}

type DetailedRetriever interface {
    RetrieveDetailed(query Query) ([]RetrievedChunk, error)
}

type Reranker interface {
    Rerank(query Query, chunks []RetrievedChunk, topK int) ([]RetrievedChunk, error)
}
```

理解方式：

- `Retriever` 是旧接口，给 Agent 使用，只返回字符串上下文。
- `DetailedRetriever` 是新接口，返回带来源、分数、通道、rank 的结果，适合评测和融合。
- `Reranker` 是重排接口，允许把 reranker 换成本地模型、HTTP 服务或第三方 API。

新增核心数据结构：

```go
type Query struct {
    Destination  string
    Preferences  []string
    Pace         string
    SpecialNotes string
    Text         string
    TopK         int
}

type RetrievedChunk struct {
    ID       string
    Title    string
    Text     string
    Source   string
    Metadata map[string]any
    Scores   map[string]float64
    Channels []string
    Rank     int
}
```

这里能学到 Go 的几个点：

- `map[string]any` 表示灵活元数据。
- `Scores` 用 map 保存不同通道分数，例如 `dense`、`lexical`、`rrf`、`rerank`。
- `Channels []string` 标记结果来自哪些检索通道。

### 6.3 Markdown 检索升级

代码位置：`internal/infrastructure/rag/markdown_retriever.go`。

新增重点：

- Markdown chunk 有稳定 `ID`。
- 自动从文件名推断城市，例如 `xiamen_guide.md` -> `xiamen`。
- 词法检索从简单字符串命中升级为 token scoring。
- 中文使用 Han 字 bigram / trigram，英文使用 ASCII token。
- 返回 `RetrievedChunk`，通道名是 `lexical`。

这部分适合学习：

- 字符串处理：`strings`。
- Unicode 判断：`unicode.Is(unicode.Han, r)`。
- 词频统计：`map[string]int`。
- 排序：`sort.SliceStable`。
- 稳定 ID：`crypto/sha1`。

### 6.4 Qdrant 检索升级

代码位置：`internal/infrastructure/rag/qdrant_retriever.go`。

新增重点：

- 支持 query variants。
- 每个 query variant 分别 embedding 和检索。
- 多 variant 结果按 chunk key 合并去重。
- 返回 `RetrievedChunk`，通道名是 `dense`。

这部分适合学习：

- 适配器模式。
- HTTP 客户端封装。
- 结果合并去重。
- 用 `map` 做聚合。

### 6.5 Hybrid 检索

代码位置：`internal/infrastructure/rag/hybrid_retriever.go`。

链路：

```text
Query
 -> dense recall: QdrantRetriever
 -> lexical recall: MarkdownRetriever
 -> RRF fusion
 -> optional HTTP reranker
 -> pack contexts
```

关键概念：

- `candidateK`：每路召回候选数。
- `finalK`：最终返回给 Agent 的条数。
- `RRF`：Reciprocal Rank Fusion，用排名而不是原始分数融合多路结果。
- `maxContextChars`：限制最终注入 Planner 的上下文总长度。

为什么要这样设计：

- dense 检索擅长语义相似。
- lexical 检索擅长关键词、城市名、景点名精确匹配。
- RRF 能避免某一路分数尺度支配整体排序。
- reranker 可以进一步用跨编码器模型判断 query-document 相关性。

## 7. 新增功能二：HTTP reranker sidecar

Go 代码位置：

- `internal/rag/retriever.go`：定义 `Reranker` 接口。
- `internal/infrastructure/rag/http_reranker.go`：实现 HTTP reranker client。
- `backend-go/scripts/reranker_server.py`：本地 Python sidecar 示例。

配置：

```env
RAG_RERANKER_URL=http://127.0.0.1:9001/rerank
RAG_RERANKER_MODEL=bge-reranker-v2-m3
RAG_RERANKER_TIMEOUT_SECONDS=30
```

启动示例：

```powershell
cd F:\Code\Travel-Agent\backend-go
python -m venv .venv-reranker
.\.venv-reranker\Scripts\Activate.ps1
pip install fastapi uvicorn FlagEmbedding
uvicorn scripts.reranker_server:app --host 127.0.0.1 --port 9001
```

Go 与 Python 之间的协议很小：

```json
{
  "query": "xiamen sunset food itinerary",
  "top_k": 5,
  "documents": [
    {"index": 0, "id": "chunk-a", "text": "document text"}
  ]
}
```

返回：

```json
{
  "results": [
    {"index": 0, "score": 0.93}
  ]
}
```

这种设计很重要：Go 后端不直接依赖 Python 模型库。你只要保持 HTTP 协议不变，就能把 reranker 换成 FlagEmbedding、Qwen reranker、云 API 或自己的模型服务。

## 8. 新增功能三：RAG 索引、评测和对比

### 8.1 索引任务

代码位置：`backend-go/cmd/index-rag/main.go`。

运行：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/index-rag
```

它会执行：

```text
读取 data/guides/*.md
 -> 切分 Markdown chunk
 -> 调 Ollama embedding
 -> 校验 embedding 维度
 -> 确保 Qdrant collection 存在
 -> 用稳定 UUID 写入 Qdrant
```

学习重点：

- 命令行程序也是 `package main`。
- `context.Background()` 用于后台任务。
- 批量写入用 `batch := make([]QdrantPoint, 0, batchSize)`。
- 稳定 UUID 用 SHA1 派生，方便重复索引和去重。

### 8.2 单后端评测

代码位置：`backend-go/cmd/eval-rag/main.go`。

运行：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/eval-rag --backend markdown --cases data\eval\rag_cases.jsonl --top-k 5
go run ./cmd/eval-rag --backend hybrid --cases data\eval\rag_cases.jsonl --top-k 5
```

输出 JSON 报告：

```powershell
go run ./cmd/eval-rag --backend hybrid --cases data\eval\rag_cases.jsonl --top-k 5 --output data\eval\hybrid-report.json
```

评测指标：

- `hit@k`：top-k 内是否命中相关结果。
- `recall@k`：相关内容召回比例。
- `mrr@k`：第一个相关结果越靠前越好。
- `ndcg@k`：综合考虑相关性和排序位置。
- `forbidden_hits`：命中不该出现的城市或内容次数。
- `latency_ms`：检索耗时。

样例数据位置：`backend-go/data/eval/rag_cases.jsonl`。

### 8.3 多配置对比

代码位置：`backend-go/cmd/compare-rag/main.go`。

它用于比较多套 RAG profile，例如：

- markdown baseline。
- qdrant dense。
- hybrid without reranker。
- hybrid with reranker。
- 不同 `candidate_k`、`rrf_k`、`query_variants`。

运行方式：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5
```

如果要输出报告：

```powershell
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5 --output data\eval\compare-report.json
```

注意：当前仓库里如果还没有 `data/eval/rag_profiles.json`，可以按 `cmd/compare-rag` 中的 `ragProfile` 结构补一个 profiles 文件。

## 9. 新增功能四：攻略语料和评测集扩展

当前攻略语料从 5 个城市扩展到更多城市：

```text
beijing_guide.md
chengdu_guide.md
chongqing_guide.md
dali_guide.md
guilin_guide.md
hangzhou_guide.md
sanya_guide.md
shanghai_guide.md
xiamen_guide.md
xian_guide.md
```

对应评测集在：

```text
backend-go/data/eval/rag_cases.jsonl
```

学习方式：

1. 先读一个 guide Markdown，理解 chunk 如何被切分。
2. 找到对应 eval case，看 `must_contain`、`forbidden`、`relevant_sources`。
3. 运行 `eval-rag` 看该 case 是否命中。
4. 如果失败，先判断是语料缺失、切分问题、检索词问题，还是排序问题。
5. 再决定改 Markdown、改 tokenizer、调 RRF，或启用 reranker。

## 10. 新增配置项

后端常用配置：

```env
PORT=8000
DATA_DIR=data/guides
STORAGE_FILE=data/trips.json

LLM_API_KEY=
LLM_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
LLM_MODEL=qwen-max
LLM_TIMEOUT_SECONDS=60

RAG_BACKEND=markdown
RAG_CANDIDATE_K=40
RAG_RRF_K=60
RAG_QUERY_VARIANTS=3
RAG_MAX_CONTEXT_CHARS=6000
RAG_RERANKER_URL=
RAG_RERANKER_MODEL=bge-reranker-v2-m3
RAG_RERANKER_TIMEOUT_SECONDS=30

QDRANT_URL=http://127.0.0.1:6333
QDRANT_COLLECTION=travel_guides
EMBEDDING_BASE_URL=http://127.0.0.1:11434
EMBEDDING_MODEL=bge-m3
EMBEDDING_DIM=1024

ENABLE_AMAP_ENRICHMENT=false
ENABLE_AMAP_WEATHER=false
ENABLE_AMAP_ROUTING=false
AMAP_API_KEY=
AMAP_BASE_URL=https://restapi.amap.com/v3
AMAP_BASE_V5_URL=https://restapi.amap.com/v5

ENABLE_WEB_RESEARCH=false
WEB_SEARCH_ENDPOINT=
WEB_SEARCH_API_KEY=
WEB_RESEARCH_TIMEOUT_SECONDS=20
WEB_RESEARCH_MAX_PAGES=3
```

理解配置读取：

- `internal/config/config.go` 定义 `Config`。
- `Load()` 读取 `.env` 和环境变量。
- `envInt`、`envBool` 将字符串转成数字和布尔值。
- `bootstrap.NewApp(cfg)` 把配置注入到各个服务。

## 11. 学习路线

### 第一阶段：跑通项目

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/server
```

访问：

```text
http://127.0.0.1:8000/health
```

再启动前端：

```powershell
cd F:\Code\Travel-Agent\frontend
npm install
npm run dev
```

### 第二阶段：读懂基础 Go 后端

按这个顺序读：

1. `cmd/server/main.go`
2. `internal/config/config.go`
3. `internal/bootstrap/app.go`
4. `internal/domain/models.go`
5. `internal/transport/http/router.go`
6. `internal/application/trip_usecase.go`
7. `internal/agent/agent.go`
8. `internal/services/planner.go`
9. `internal/infrastructure/storage/json_repository.go`

目标是看懂：

- 请求如何进入。
- 数据结构如何定义。
- 接口如何隔离实现。
- Repository 如何保存数据。
- Agent 如何产出 itinerary。

### 第三阶段：读懂 RAG

按这个顺序读：

1. `internal/rag/retriever.go`
2. `internal/infrastructure/rag/markdown_retriever.go`
3. `internal/infrastructure/rag/qdrant_client.go`
4. `internal/infrastructure/rag/embedding_client.go`
5. `internal/infrastructure/rag/qdrant_retriever.go`
6. `internal/infrastructure/rag/hybrid_retriever.go`
7. `internal/infrastructure/rag/http_reranker.go`
8. `cmd/eval-rag/main.go`
9. `cmd/compare-rag/main.go`

目标是看懂：

- 为什么要从 `Retriever` 升级到 `DetailedRetriever`。
- dense、lexical、RRF、reranker 分别解决什么问题。
- 评测指标如何计算。
- 如何用数据驱动 RAG 调优。

### 第四阶段：做一个小改动

建议练习：

1. 给 `TripRequest` 新增一个字段，例如 `TransportationPreference`。
2. 修改前端类型和表单。
3. 在 `Query.SearchText()` 或 Planner prompt 中使用它。
4. 增加一个 eval case 验证检索是否能召回相关攻略。
5. 运行：

```powershell
cd F:\Code\Travel-Agent\backend-go
go test ./...
go run ./cmd/eval-rag --backend markdown --cases data\eval\rag_cases.jsonl --top-k 5
```

这个练习会同时覆盖 Go struct、JSON tag、前后端字段同步、RAG query 构造和测试验证。

## 12. 新增功能时应该放在哪里

新增 HTTP 接口：

```text
domain 定义请求/响应结构
application 增加 usecase 方法
transport/http 增加路由 handler
```

新增外部 API：

```text
internal/<area> 定义小接口
internal/infrastructure/<area> 写具体 HTTP client
internal/config 加配置
internal/bootstrap 装配
```

新增 Agent 工具：

```text
services 或 infrastructure 实现能力
tools 包装成工具
agent/tool_models.go 定义工具参数和结果
agent/tool_calling_agent.go 注册工具 schema 和执行逻辑
```

新增 RAG 后端：

```text
internal/rag/retriever.go 确认接口
internal/infrastructure/rag 新增实现
internal/bootstrap/app.go 增加 RAG_BACKEND 分支
cmd/eval-rag 增加可评测路径
data/eval/rag_cases.jsonl 增加回归样例
```

## 13. 验证命令

后端测试：

```powershell
cd F:\Code\Travel-Agent\backend-go
go test ./...
```

RAG 索引：

```powershell
go run ./cmd/index-rag
```

RAG 单后端评测：

```powershell
go run ./cmd/eval-rag --backend markdown --cases data\eval\rag_cases.jsonl --top-k 5
go run ./cmd/eval-rag --backend hybrid --cases data\eval\rag_cases.jsonl --top-k 5
```

RAG 多配置对比：

```powershell
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5
```

前端构建：

```powershell
cd F:\Code\Travel-Agent\frontend
npm run build
```

## 14. 这轮新增功能的学习关键词

- `DetailedRetriever`：让检索结果携带更多元数据。
- `RetrievedChunk`：检索结果的统一表达。
- `Query`：统一不同 RAG 后端的查询输入。
- `Reranker`：重排能力接口。
- `HybridRetriever`：组合多个检索后端。
- `RRF`：多路排序融合算法。
- `HTTPReranker`：Go 调外部模型 sidecar。
- `eval-rag`：检索质量自动评测。
- `compare-rag`：多套 RAG 配置横向比较。
- `rag_cases.jsonl`：把人工判断沉淀成回归测试集。

掌握这几个词，就能读懂这轮新增功能的主干。
