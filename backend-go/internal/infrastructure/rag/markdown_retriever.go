package rag

import base "zhilv-yuntu-go/internal/rag"

// MarkdownRetriever 是基础设施层的本地 Markdown 检索实现。
// 后续接 Chroma、Elasticsearch 或向量数据库时，可以新增同接口实现。
type MarkdownRetriever = base.MarkdownRetriever

func NewMarkdownRetriever(dataDir string) *MarkdownRetriever {
	return base.NewMarkdownRetriever(dataDir)
}
