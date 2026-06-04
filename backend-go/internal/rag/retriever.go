package rag

import "strings"

// Retriever 是 RAG 检索接口。
// Go 里通常会把“能力”抽象成小接口：谁需要检索，就依赖 Retriever。
type Retriever interface {
	Retrieve(destination string, preferences []string, pace string, specialNotes string, topK int) ([]string, error)
}

type Chunk struct {
	Title  string
	Text   string
	Source string
}

func buildQuery(destination string, preferences []string, pace string, specialNotes string) []string {
	parts := []string{destination, "景点", "行程", "攻略", "推荐"}
	parts = append(parts, preferences...)
	if pace != "" {
		parts = append(parts, pace)
	}
	if strings.Contains(specialNotes, "日落") {
		parts = append(parts, "日落", "傍晚", "拍照")
	}
	if strings.Contains(specialNotes, "不想太早") || strings.Contains(specialNotes, "自然醒") {
		parts = append(parts, "轻松", "慢节奏")
	}
	return uniqueNonEmpty(parts)
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
