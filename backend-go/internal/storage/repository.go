package storage

import "zhilv-yuntu-go/internal/domain"

// TripRepository 是“仓储模式”的接口。
// 业务层只依赖这个接口，而不关心底层是 JSON 文件、SQLite、MySQL 还是云数据库。
// 这就是 Go 里很常见的依赖倒置：上层依赖抽象，下层实现细节。
type TripRepository interface {
	Save(itinerary domain.Itinerary) (string, error)
	Get(tripID string) (domain.TripDetailResponse, bool, error)
	List() (domain.TripListResponse, error)
	Delete(tripID string) (bool, error)
}
