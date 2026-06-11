# 高级 RAG 搭建说明

项目现在支持一套更接近生产形态的 RAG 链路：

```text
用户查询
 -> Qdrant dense 向量召回
 -> 本地 Markdown 词法召回
 -> RRF 融合
 -> 可选 HTTP 重排模型
 -> 上下文打包
 -> Agent / Planner
```

实现上仍然兼容旧的 `rag.Retriever` 接口，所以 Agent 层继续接收
`[]string` 上下文。更丰富的检索元数据通过 `rag.DetailedRetriever`
暴露，用于融合、重排和自动化评测。

## 检索后端

通过 `RAG_BACKEND` 选择检索路径：

```env
RAG_BACKEND=markdown  # 只使用本地 Markdown 词法检索
RAG_BACKEND=qdrant    # 只使用 Qdrant dense 向量检索
RAG_BACKEND=hybrid    # dense + lexical + RRF + 可选 reranker
```

推荐的高级 RAG 配置：

```env
DATA_DIR=data/guides
RAG_BACKEND=hybrid
RAG_CANDIDATE_K=40
RAG_RRF_K=60
RAG_QUERY_VARIANTS=3
RAG_MAX_CONTEXT_CHARS=6000

QDRANT_URL=http://127.0.0.1:6333
QDRANT_COLLECTION=travel_guides

EMBEDDING_BASE_URL=http://127.0.0.1:11434
EMBEDDING_MODEL=bge-m3
EMBEDDING_DIM=1024
```

几个关键参数的含义：

- `RAG_CANDIDATE_K`：每路召回进入融合前的候选数量。
- `RAG_RRF_K`：RRF 融合的平滑参数，常用值是 60。
- `RAG_QUERY_VARIANTS`：dense 召回时使用的查询变体数量。
- `RAG_MAX_CONTEXT_CHARS`：最终注入 Planner 的上下文最大字符数。

## 索引流程

先启动 Qdrant，并确保 embedding 服务可用，然后执行索引任务：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/index-rag
```

索引任务会把 Markdown 攻略切成 chunk，并写入 Qdrant。每个 payload 会包含稳定的
`chunk_id`、`title`、`source`、`text` 和 metadata。稳定 ID 对去重、融合和评测都很重要。

## 重排模型

Go 服务可以调用一个可选的 HTTP reranker。如果没有配置 `RAG_RERANKER_URL`，
`hybrid` 后端会自动退回到只使用 RRF 排序。

```env
RAG_RERANKER_URL=http://127.0.0.1:9001/rerank
RAG_RERANKER_MODEL=bge-reranker-v2-m3
RAG_RERANKER_TIMEOUT_SECONDS=30
```

本地重排 sidecar 示例：

```powershell
cd F:\Code\Travel-Agent\backend-go
python -m venv .venv-reranker
.\.venv-reranker\Scripts\Activate.ps1
pip install fastapi uvicorn FlagEmbedding
uvicorn scripts.reranker_server:app --host 127.0.0.1 --port 9001
```

sidecar 接收的请求格式：

```json
{
  "query": "xiamen sunset food itinerary",
  "top_k": 5,
  "documents": [
    {"index": 0, "id": "chunk-a", "text": "document text"}
  ]
}
```

sidecar 返回的结果格式：

```json
{
  "results": [
    {"index": 0, "score": 0.93}
  ]
}
```

这样设计的好处是：Go 后端不需要直接依赖 Python 模型库，重排模型可以用
FlagEmbedding、Qwen reranker 或第三方 rerank API 包装成同一个 HTTP 协议。

## 自动化评测

使用内置样例集运行检索评测：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/eval-rag --backend markdown --cases data\eval\rag_cases.jsonl --top-k 5
go run ./cmd/eval-rag --backend hybrid --cases data\eval\rag_cases.jsonl --top-k 5
```

输出 JSON 报告：

```powershell
go run ./cmd/eval-rag --backend hybrid --cases data\eval\rag_cases.jsonl --top-k 5 --output data\eval\hybrid-report.json
```

当前评测命令会输出：

- `hit@k`
- `recall@k`
- `mrr@k`
- `ndcg@k`
- 禁用词命中次数
- 平均延迟
- 每个样例命中的召回通道

样例 JSONL 格式：

```json
{
  "id": "xiamen_sunset_food",
  "destination": "xiamen",
  "preferences": ["food", "coast"],
  "pace": "relaxed",
  "special_notes": "sunset",
  "query": "xiamen relaxed coast sunset local food",
  "top_k": 5,
  "relevant_sources": ["xiamen_guide.md"],
  "must_contain": ["环岛路", "日落", "沙茶"],
  "forbidden": ["dali", "sanya", "chengdu", "xian"]
}
```

正式调优时，建议持续扩充 `data/eval/rag_cases.jsonl`，把人工确认过的失败样例、
线上错误样例和重要业务场景都加入回归集。如果能标注稳定的 `relevant_chunk_ids`，
检索层指标会更可靠。

RAGAS 更适合后续补在生成层，用来评估回答忠实性、上下文相关性和答案相关性。
本项目当前的 `cmd/eval-rag` 先负责检索和重排层的硬指标。

## 效果对比 / A/B 实验

如果要比较“旧版简单 RAG”和“高级 RAG”，或者比较高级 RAG 使用不同 embedding / reranker 模型时的效果，不要手工改 `.env` 后一遍遍跑单次评测。项目提供了 profile 化的对比命令：

```powershell
cd F:\Code\Travel-Agent\backend-go
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5
```

输出 JSON 对比报告：

```powershell
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5 --output data\eval\rag-compare-report.json
```

同时保存中文 Markdown 实验报告：

```powershell
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.json --top-k 5 --output data\eval\rag-compare-report.json --markdown-output ..\docs\rag-experiment-report.md
```

建议把 JSON 报告当作机器可读的原始结果，把 Markdown 报告当作人工复盘文档。Markdown 报告会包含实验配置、汇总排名、关键观察和逐 case 明细。

`data/eval/rag_profiles.json` 中第一组 profile 会被当作 baseline，后面的 profile 会自动计算相对 baseline 的变化。默认配置包含：

- `simple_markdown`：旧版/简单 RAG，只走本地 Markdown 词法检索。
- `qdrant_bge_m3`：只走 Qdrant dense 向量召回。
- `hybrid_bge_m3_rrf`：高级 RAG，dense + lexical + RRF，不启用重排。
- `hybrid_bge_m3_reranker`：高级 RAG，dense + lexical + RRF + HTTP reranker。

对比报告会记录每组 profile 的 `backend`、`qdrant_collection`、`embedding_model`、`embedding_dim`、`reranker_model`、`reranked_cases`、`hit_rate`、`average_recall`、`average_mrr`、`average_ndcg`、`average_latency_ms`、`quality_score` 和相对 baseline 的 delta。`reranker_enabled` 表示 profile 配置了 reranker，`reranked_cases` 表示有多少条样例的结果真的经过了 `rerank` 通道；如果 reranker sidecar 没启动，这个值通常会是 0。`quality_score` 是一个综合分：优先看召回和排序质量，同时惩罚禁用词命中和失败样例。它适合做快速排序，但最终判断仍然要结合逐 case 报告看失败原因。

### 对比不同 embedding 模型

embedding 模型对比必须保证“检索时使用的模型”和“索引时写入 Qdrant 的向量模型”一致。只改 `embedding_model` 而不重建索引是不公平的，因为 Qdrant collection 里的向量仍然来自旧模型，维度也可能不一致。

推荐做法是每个 embedding 模型使用独立 collection：

```env
QDRANT_COLLECTION=travel_guides_bge_m3
EMBEDDING_MODEL=bge-m3
EMBEDDING_DIM=1024
```

```powershell
go run ./cmd/index-rag
```

然后切换另一套配置并重新索引：

```env
QDRANT_COLLECTION=travel_guides_bge_large_zh
EMBEDDING_MODEL=bge-large-zh
EMBEDDING_DIM=1024
```

```powershell
go run ./cmd/index-rag
```

索引都准备好后，可以参考 `data/eval/rag_profiles.models.example.json`，把不同 collection 写成不同 profile，再运行：

```powershell
go run ./cmd/compare-rag --cases data\eval\rag_cases.jsonl --profiles data\eval\rag_profiles.models.example.json --top-k 5
```

### 对比不同 reranker 模型

reranker 对比最好复用同一个召回配置和同一个 Qdrant collection，只改变 `reranker_url` / `reranker_model`。这样可以把变量控制在“重排模型”本身，而不是把 embedding、索引和召回参数混在一起。

当前示例 sidecar 在启动时加载一个模型，所以对比多个 reranker 时可以启动多个端口：

```powershell
$env:RERANKER_MODEL="BAAI/bge-reranker-v2-m3"
uvicorn scripts.reranker_server:app --host 127.0.0.1 --port 9001
```

另开一个终端：

```powershell
$env:RERANKER_MODEL="your-qwen-reranker-model"
uvicorn scripts.reranker_server:app --host 127.0.0.1 --port 9002
```

然后在 profile 中分别配置 `http://127.0.0.1:9001/rerank` 和 `http://127.0.0.1:9002/rerank`。如果后续希望一个 sidecar 同时支持多个模型，可以把 `scripts/reranker_server.py` 扩展为按请求里的 `model` 字段做模型缓存。

### 评测设计方法

设计 RAG 对比实验时要控制变量：

1. 固定同一份 `rag_cases.jsonl`，不要在一次对比中途增删样例。
2. 固定 `top-k`，否则 recall、MRR、nDCG 不可直接比较。
3. embedding 对比时，每个模型使用独立 collection，并确认索引已重建。
4. reranker 对比时，尽量使用同一个 embedding collection 和同一组 `candidate_k` / `rrf_k`。
5. 每次线上发现错误，把失败问题加入 `rag_cases.jsonl`，让评测集逐步覆盖真实业务场景。

## 代码落点

- `internal/rag/retriever.go`：定义旧版 `Retriever`，以及新版 `DetailedRetriever` / `Reranker`。
- `internal/infrastructure/rag/markdown_retriever.go`：本地 Markdown 词法召回。
- `internal/infrastructure/rag/qdrant_retriever.go`：Qdrant dense 向量召回。
- `internal/infrastructure/rag/hybrid_retriever.go`：多路召回、RRF 融合和上下文打包。
- `internal/infrastructure/rag/http_reranker.go`：HTTP 重排模型客户端。
- `cmd/eval-rag`：RAG 自动化评测命令。
- `cmd/compare-rag`：RAG 多 profile 效果对比命令。
- `data/eval/rag_profiles.json`：默认效果对比 profile。
- `data/eval/rag_profiles.models.example.json`：不同 embedding / reranker 模型的 A/B 示例 profile。
- `scripts/reranker_server.py`：本地重排模型 sidecar 示例。

## 推荐迭代方式

1. 先用 `markdown` 跑通评测，确认样例集格式正确。
2. 启动 Qdrant 和 embedding 服务，运行 `cmd/index-rag`。
3. 用 `qdrant` 跑评测，观察 dense 召回表现。
4. 切到 `hybrid`，比较 RRF 融合后的 `recall@k`、`mrr@k` 和 `ndcg@k`。
5. 启动 reranker sidecar，再比较重排前后的指标变化。
6. 用 `cmd/compare-rag` 把旧版 RAG、高级 RAG 和不同模型 profile 放在同一张表里比较。
7. 把失败样例持续加入 `data/eval/rag_cases.jsonl`，作为回归测试集。
