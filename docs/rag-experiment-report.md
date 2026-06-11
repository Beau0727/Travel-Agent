# RAG 效果对比实验报告

## 实验配置

- 生成时间：2026-06-08T11:12:24+08:00
- 评测样例：`data\eval\rag_cases.jsonl`
- Profile 配置：`data\eval\rag_profiles.json`
- Top K：5
- Baseline：`simple_markdown`
- Profile 数量：4
- Case 数量：10

## 汇总结果

| 排名 | Profile | Backend | Collection | Embedding | Reranker | 实际重排样例 | Quality | ΔQuality | Recall | MRR | nDCG | 平均延迟(ms) | 失败样例 |
|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | `simple_markdown` | `markdown` | - | - | 未启用 | 0 | 1.000 | +0.000 | 1.000 | 1.000 | 1.000 | 13.3 | 0 |
| 2 | `hybrid_bge_m3_rrf` | `hybrid` | `travel_guides` | `bge-m3/1024` | 未启用 | 0 | 0.601 | -0.399 | 1.000 | 0.950 | 0.903 | 645.6 | 0 |
| 3 | `hybrid_bge_m3_reranker` | `hybrid` | `travel_guides` | `bge-m3/1024` | `bge-reranker-v2-m3` | 0 | 0.601 | -0.399 | 1.000 | 0.950 | 0.903 | 649.5 | 0 |
| 4 | `qdrant_bge_m3` | `qdrant` | `travel_guides` | `bge-m3/1024` | 未启用 | 0 | 0.000 | -1.000 | 0.500 | 0.400 | 0.473 | 1809.7 | 0 |

## 关键观察

- 当前综合分最高的是 `simple_markdown`，quality_score=1.000，平均延迟 13.3 ms。
- Baseline `simple_markdown` 的 quality_score=1.000，后续 profile 的 ΔQuality 都是相对它计算。
- `hybrid_bge_m3_reranker` 配置了 reranker `bge-reranker-v2-m3`，但 `reranked_cases=0`，通常说明 reranker 服务没有启动、请求失败，或返回结果没有进入 `rerank` 通道。

## Profile：simple_markdown

- 说明：旧版/简单 RAG：只使用本地 Markdown 词法检索
- Backend：`markdown`
- Reranker：未启用，实际重排样例：0/10
- 指标：hit_rate=1.000，recall=1.000，mrr=1.000，ndcg=1.000，quality=1.000，平均延迟=13.3 ms

| Case | Hit | Recall | MRR | nDCG | 延迟(ms) | 通道 | Top Sources | 错误 |
|---|---:|---:|---:|---:|---:|---|---|---|
| `xiamen_sunset_food` | true | 1.000 | 1.000 | 1.000 | 14 | lexical | xiamen_guide.md | - |
| `dali_old_town_lake` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | dali_guide.md | - |
| `chengdu_food_panda` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | chengdu_guide.md | - |
| `sanya_family_beach` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | sanya_guide.md | - |
| `xian_history_food` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | xian_guide.md | - |
| `beijing_first_time_history` | true | 1.000 | 1.000 | 1.000 | 12 | lexical | beijing_guide.md | - |
| `shanghai_citywalk_night` | true | 1.000 | 1.000 | 1.000 | 16 | lexical | shanghai_guide.md | - |
| `hangzhou_westlake_tea` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | hangzhou_guide.md | - |
| `chongqing_food_night` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | chongqing_guide.md | - |
| `guilin_landscape_yangshuo` | true | 1.000 | 1.000 | 1.000 | 13 | lexical | guilin_guide.md | - |

## Profile：hybrid_bge_m3_rrf

- 说明：高级 RAG：dense + lexical + RRF，不启用重排
- Backend：`hybrid`
- Qdrant Collection：`travel_guides`
- Embedding：`bge-m3`，维度：1024
- Reranker：未启用，实际重排样例：0/10
- 指标：hit_rate=1.000，recall=1.000，mrr=0.950，ndcg=0.903，quality=0.601，平均延迟=645.6 ms

| Case | Hit | Recall | MRR | nDCG | 延迟(ms) | 通道 | Top Sources | 错误 |
|---|---:|---:|---:|---:|---:|---|---|---|
| `xiamen_sunset_food` | true | 1.000 | 0.500 | 0.712 | 681 | dense<br>lexical | sanya_guide.md<br>xiamen_guide.md<br>xian_guide.md | - |
| `dali_old_town_lake` | true | 1.000 | 1.000 | 1.000 | 667 | lexical<br>dense | dali_guide.md | - |
| `chengdu_food_panda` | true | 1.000 | 1.000 | 0.885 | 690 | lexical<br>dense | chengdu_guide.md<br>xian_guide.md | - |
| `sanya_family_beach` | true | 1.000 | 1.000 | 1.000 | 636 | dense<br>lexical | sanya_guide.md | - |
| `xian_history_food` | true | 1.000 | 1.000 | 1.000 | 634 | lexical<br>dense | xian_guide.md<br>chengdu_guide.md | - |
| `beijing_first_time_history` | true | 1.000 | 1.000 | 0.885 | 544 | lexical<br>dense | beijing_guide.md<br>xian_guide.md | - |
| `shanghai_citywalk_night` | true | 1.000 | 1.000 | 0.877 | 594 | lexical<br>dense | shanghai_guide.md<br>xian_guide.md | - |
| `hangzhou_westlake_tea` | true | 1.000 | 1.000 | 0.877 | 616 | lexical<br>dense | hangzhou_guide.md<br>sanya_guide.md<br>chengdu_guide.md | - |
| `chongqing_food_night` | true | 1.000 | 1.000 | 0.906 | 766 | lexical<br>dense | chongqing_guide.md<br>xian_guide.md<br>chengdu_guide.md | - |
| `guilin_landscape_yangshuo` | true | 1.000 | 1.000 | 0.885 | 628 | lexical<br>dense | guilin_guide.md<br>dali_guide.md<br>chengdu_guide.md | - |

## Profile：hybrid_bge_m3_reranker

- 说明：高级 RAG：dense + lexical + RRF + bge reranker
- Backend：`hybrid`
- Qdrant Collection：`travel_guides`
- Embedding：`bge-m3`，维度：1024
- Reranker：`bge-reranker-v2-m3`，实际重排样例：0/10
- 指标：hit_rate=1.000，recall=1.000，mrr=0.950，ndcg=0.903，quality=0.601，平均延迟=649.5 ms

| Case | Hit | Recall | MRR | nDCG | 延迟(ms) | 通道 | Top Sources | 错误 |
|---|---:|---:|---:|---:|---:|---|---|---|
| `xiamen_sunset_food` | true | 1.000 | 0.500 | 0.712 | 659 | dense<br>lexical | sanya_guide.md<br>xiamen_guide.md<br>xian_guide.md | - |
| `dali_old_town_lake` | true | 1.000 | 1.000 | 1.000 | 610 | lexical<br>dense | dali_guide.md | - |
| `chengdu_food_panda` | true | 1.000 | 1.000 | 0.885 | 615 | lexical<br>dense | chengdu_guide.md<br>xian_guide.md | - |
| `sanya_family_beach` | true | 1.000 | 1.000 | 1.000 | 553 | dense<br>lexical | sanya_guide.md | - |
| `xian_history_food` | true | 1.000 | 1.000 | 1.000 | 584 | lexical<br>dense | xian_guide.md<br>chengdu_guide.md | - |
| `beijing_first_time_history` | true | 1.000 | 1.000 | 0.885 | 712 | lexical<br>dense | beijing_guide.md<br>xian_guide.md | - |
| `shanghai_citywalk_night` | true | 1.000 | 1.000 | 0.877 | 748 | lexical<br>dense | shanghai_guide.md<br>xian_guide.md | - |
| `hangzhou_westlake_tea` | true | 1.000 | 1.000 | 0.877 | 692 | lexical<br>dense | hangzhou_guide.md<br>sanya_guide.md<br>chengdu_guide.md | - |
| `chongqing_food_night` | true | 1.000 | 1.000 | 0.906 | 700 | lexical<br>dense | chongqing_guide.md<br>xian_guide.md<br>chengdu_guide.md | - |
| `guilin_landscape_yangshuo` | true | 1.000 | 1.000 | 0.885 | 622 | lexical<br>dense | guilin_guide.md<br>dali_guide.md<br>chengdu_guide.md | - |

## Profile：qdrant_bge_m3

- 说明：纯 dense 向量召回，embedding=bge-m3
- Backend：`qdrant`
- Qdrant Collection：`travel_guides`
- Embedding：`bge-m3`，维度：1024
- Reranker：未启用，实际重排样例：0/10
- 指标：hit_rate=0.600，recall=0.500，mrr=0.400，ndcg=0.473，quality=0.000，平均延迟=1809.7 ms

| Case | Hit | Recall | MRR | nDCG | 延迟(ms) | 通道 | Top Sources | 错误 |
|---|---:|---:|---:|---:|---:|---|---|---|
| `xiamen_sunset_food` | true | 0.667 | 0.333 | 0.544 | 12488 | dense | sanya_guide.md<br>xian_guide.md<br>xiamen_guide.md | - |
| `dali_old_town_lake` | true | 1.000 | 1.000 | 1.000 | 643 | dense | dali_guide.md | - |
| `chengdu_food_panda` | true | 1.000 | 0.333 | 0.618 | 651 | dense | xian_guide.md<br>dali_guide.md<br>chengdu_guide.md | - |
| `sanya_family_beach` | true | 1.000 | 1.000 | 1.000 | 723 | dense | sanya_guide.md<br>xiamen_guide.md | - |
| `xian_history_food` | true | 1.000 | 1.000 | 1.000 | 617 | dense | xian_guide.md<br>chengdu_guide.md | - |
| `beijing_first_time_history` | false | 0.000 | 0.000 | 0.000 | 624 | dense | xian_guide.md<br>chengdu_guide.md | - |
| `shanghai_citywalk_night` | false | 0.000 | 0.000 | 0.000 | 615 | dense | xian_guide.md<br>chengdu_guide.md | - |
| `hangzhou_westlake_tea` | false | 0.000 | 0.000 | 0.000 | 542 | dense | sanya_guide.md<br>chengdu_guide.md<br>xiamen_guide.md | - |
| `chongqing_food_night` | true | 0.333 | 0.333 | 0.571 | 604 | dense | xian_guide.md<br>chengdu_guide.md<br>dali_guide.md<br>xiamen_guide.md | - |
| `guilin_landscape_yangshuo` | false | 0.000 | 0.000 | 0.000 | 590 | dense | dali_guide.md<br>chengdu_guide.md | - |

