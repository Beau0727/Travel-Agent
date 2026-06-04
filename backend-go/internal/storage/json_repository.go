package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// JSONTripRepository 是 TripRepository 的一个教学版实现。
// Python 版用 SQLite；Go 版这里先用 JSON 文件，是为了让你少受数据库驱动干扰，
// 把注意力放在接口、结构体、错误处理和 HTTP 服务上。
type JSONTripRepository struct {
	filePath string
	mu       sync.Mutex
}

type tripRecord struct {
	TripID      string           `json:"trip_id"`
	Destination string           `json:"destination"`
	Summary     string           `json:"summary"`
	Itinerary   domain.Itinerary `json:"itinerary"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
}

func NewJSONTripRepository(filePath string) *JSONTripRepository {
	return &JSONTripRepository{filePath: filePath}
}

func (r *JSONTripRepository) Save(itinerary domain.Itinerary) (string, error) {
	start := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.load()
	if err != nil {
		logging.Error(nil, "trip repository save load failed",
			"trip_id", itinerary.TripID,
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", err
	}

	now := time.Now().Format(time.RFC3339)
	record, exists := records[itinerary.TripID]
	if !exists {
		record.CreatedAt = now
	}
	record.TripID = itinerary.TripID
	record.Destination = itinerary.Destination
	record.Summary = itinerary.Summary
	record.Itinerary = itinerary
	record.UpdatedAt = now
	records[itinerary.TripID] = record

	if err := r.save(records); err != nil {
		logging.Error(nil, "trip repository save failed",
			"trip_id", itinerary.TripID,
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", err
	}
	logging.Info(nil, "trip repository save completed",
		"trip_id", itinerary.TripID,
		"file", r.filePath,
		"records", len(records),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return itinerary.TripID, nil
}

func (r *JSONTripRepository) Get(tripID string) (domain.TripDetailResponse, bool, error) {
	start := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.load()
	if err != nil {
		logging.Error(nil, "trip repository get failed",
			"trip_id", tripID,
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.TripDetailResponse{}, false, err
	}
	record, ok := records[tripID]
	if !ok {
		logging.Info(nil, "trip repository get completed",
			"trip_id", tripID,
			"found", false,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return domain.TripDetailResponse{}, false, nil
	}
	logging.Info(nil, "trip repository get completed",
		"trip_id", tripID,
		"found", true,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return domain.TripDetailResponse{
		TripID:    record.TripID,
		Itinerary: record.Itinerary,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, true, nil
}

func (r *JSONTripRepository) List() (domain.TripListResponse, error) {
	start := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.load()
	if err != nil {
		logging.Error(nil, "trip repository list failed",
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.TripListResponse{}, err
	}

	items := make([]domain.TripSummaryItem, 0, len(records))
	for _, record := range records {
		items = append(items, domain.TripSummaryItem{
			TripID:      record.TripID,
			Destination: record.Destination,
			Summary:     record.Summary,
			CreatedAt:   record.CreatedAt,
			UpdatedAt:   record.UpdatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})

	logging.Info(nil, "trip repository list completed",
		"file", r.filePath,
		"total", len(items),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return domain.TripListResponse{Total: len(items), Items: items}, nil
}

func (r *JSONTripRepository) Delete(tripID string) (bool, error) {
	start := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.load()
	if err != nil {
		logging.Error(nil, "trip repository delete load failed",
			"trip_id", tripID,
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return false, err
	}
	if _, ok := records[tripID]; !ok {
		logging.Info(nil, "trip repository delete completed",
			"trip_id", tripID,
			"deleted", false,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return false, nil
	}
	delete(records, tripID)
	if err := r.save(records); err != nil {
		logging.Error(nil, "trip repository delete save failed",
			"trip_id", tripID,
			"file", r.filePath,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return false, err
	}
	logging.Info(nil, "trip repository delete completed",
		"trip_id", tripID,
		"deleted", true,
		"records", len(records),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return true, nil
}

func (r *JSONTripRepository) load() (map[string]tripRecord, error) {
	records := map[string]tripRecord{}
	data, err := os.ReadFile(r.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return records, nil
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *JSONTripRepository) save(records map[string]tripRecord) error {
	if err := os.MkdirAll(filepath.Dir(r.filePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.filePath, data, 0644)
}
