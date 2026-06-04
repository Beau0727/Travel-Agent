package application

import (
	"context"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
	"zhilv-yuntu-go/internal/services"
	"zhilv-yuntu-go/internal/storage"
)

// ItineraryGenerator 是应用层需要的 Agent 能力。
// 它只描述“根据请求生成最终行程”，不关心内部是单 Agent、多 Agent 还是普通规则服务。
type ItineraryGenerator interface {
	Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error)
}

// TripUsecase 是旅行规划的应用用例层。
// Clean Architecture 中，HTTP handler 不直接编排业务细节，而是调用 usecase。
type TripUsecase struct {
	generator  ItineraryGenerator
	editor     *services.TripService
	repository storage.TripRepository
}

func NewTripUsecase(
	generator ItineraryGenerator,
	editor *services.TripService,
	repository storage.TripRepository,
) *TripUsecase {
	return &TripUsecase{
		generator:  generator,
		editor:     editor,
		repository: repository,
	}
}

func (u *TripUsecase) Generate(ctx context.Context, request domain.TripRequest) (domain.Itinerary, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase generate started",
		"destination", request.Destination,
		"start_date", request.StartDate,
		"end_date", request.EndDate,
	)
	itinerary, err := u.generator.Generate(ctx, request)
	if err != nil {
		logging.Error(ctx, "trip usecase generate failed",
			"destination", request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.Itinerary{}, err
	}
	logging.Info(ctx, "trip usecase generate completed",
		"trip_id", itinerary.TripID,
		"destination", itinerary.Destination,
		"days", len(itinerary.Days),
		"estimated_budget", itinerary.EstimatedBudget,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return itinerary, nil
}

func (u *TripUsecase) Edit(ctx context.Context, request domain.TripEditRequest) (domain.Itinerary, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase edit started",
		"trip_id", request.TripID,
		"edit_scope", request.EditScope,
		"current_days", len(request.CurrentItinerary.Days),
	)
	itinerary, err := u.editor.Edit(ctx, request)
	if err != nil {
		logging.Error(ctx, "trip usecase edit failed",
			"trip_id", request.TripID,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.Itinerary{}, err
	}
	logging.Info(ctx, "trip usecase edit completed",
		"trip_id", itinerary.TripID,
		"days", len(itinerary.Days),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return itinerary, nil
}

func (u *TripUsecase) Save(ctx context.Context, request domain.TripSaveRequest) (string, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase save started",
		"trip_id", request.Itinerary.TripID,
		"destination", request.Itinerary.Destination,
		"days", len(request.Itinerary.Days),
	)
	tripID, err := u.repository.Save(request.Itinerary)
	if err != nil {
		logging.Error(ctx, "trip usecase save failed",
			"trip_id", request.Itinerary.TripID,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", err
	}
	logging.Info(ctx, "trip usecase save completed",
		"trip_id", tripID,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return tripID, nil
}

func (u *TripUsecase) List(ctx context.Context) (domain.TripListResponse, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase list started")
	response, err := u.repository.List()
	if err != nil {
		logging.Error(ctx, "trip usecase list failed",
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.TripListResponse{}, err
	}
	logging.Info(ctx, "trip usecase list completed",
		"total", response.Total,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return response, nil
}

func (u *TripUsecase) Get(ctx context.Context, tripID string) (domain.TripDetailResponse, bool, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase get started", "trip_id", tripID)
	detail, ok, err := u.repository.Get(tripID)
	if err != nil {
		logging.Error(ctx, "trip usecase get failed",
			"trip_id", tripID,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return domain.TripDetailResponse{}, false, err
	}
	logging.Info(ctx, "trip usecase get completed",
		"trip_id", tripID,
		"found", ok,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return detail, ok, nil
}

func (u *TripUsecase) Delete(ctx context.Context, tripID string) (bool, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase delete started", "trip_id", tripID)
	deleted, err := u.repository.Delete(tripID)
	if err != nil {
		logging.Error(ctx, "trip usecase delete failed",
			"trip_id", tripID,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return false, err
	}
	logging.Info(ctx, "trip usecase delete completed",
		"trip_id", tripID,
		"deleted", deleted,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return deleted, nil
}

func (u *TripUsecase) ExportMarkdown(ctx context.Context, tripID string) (string, bool, error) {
	start := time.Now()
	logging.Info(ctx, "trip usecase export markdown started", "trip_id", tripID)
	detail, ok, err := u.repository.Get(tripID)
	if err != nil || !ok {
		if err != nil {
			logging.Error(ctx, "trip usecase export markdown failed",
				"trip_id", tripID,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)
		} else {
			logging.Warn(ctx, "trip usecase export markdown missing trip",
				"trip_id", tripID,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
		return "", ok, err
	}
	markdown := services.RenderMarkdown(detail)
	logging.Info(ctx, "trip usecase export markdown completed",
		"trip_id", tripID,
		"bytes", len(markdown),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return markdown, true, nil
}
