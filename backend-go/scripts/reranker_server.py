import os
from typing import List, Optional

from fastapi import FastAPI
from pydantic import BaseModel

try:
    from FlagEmbedding import FlagReranker
except ImportError as exc:
    raise SystemExit(
        "FlagEmbedding is required. Install with: "
        "pip install fastapi uvicorn FlagEmbedding"
    ) from exc


DEFAULT_MODEL = os.getenv("RERANKER_MODEL", "BAAI/bge-reranker-v2-m3")
USE_FP16 = os.getenv("RERANKER_USE_FP16", "true").lower() == "true"

app = FastAPI(title="Travel Agent Reranker")
reranker = FlagReranker(DEFAULT_MODEL, use_fp16=USE_FP16)


class Document(BaseModel):
    index: int
    id: Optional[str] = None
    text: str


class RerankRequest(BaseModel):
    model: Optional[str] = None
    query: str
    top_k: Optional[int] = None
    documents: List[Document]


@app.post("/rerank")
def rerank(request: RerankRequest):
    pairs = [[request.query, item.text] for item in request.documents]
    scores = reranker.compute_score(pairs, normalize=True)
    if not isinstance(scores, list):
        scores = [scores]

    results = [
        {"index": doc.index, "score": float(score)}
        for doc, score in zip(request.documents, scores)
    ]
    results.sort(key=lambda item: item["score"], reverse=True)
    if request.top_k and request.top_k > 0:
        results = results[: request.top_k]
    return {"results": results}
