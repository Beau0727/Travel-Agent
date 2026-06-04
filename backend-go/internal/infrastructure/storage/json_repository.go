package storage

import base "zhilv-yuntu-go/internal/storage"

// JSONTripRepository 是基础设施层的 JSON 仓储实现。
// 当前复用教学版 storage 包里的实现；后续可以在这里替换为 SQLite/MySQL。
type JSONTripRepository = base.JSONTripRepository

func NewJSONTripRepository(filePath string) *JSONTripRepository {
	return base.NewJSONTripRepository(filePath)
}
