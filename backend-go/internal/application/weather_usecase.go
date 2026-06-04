package application

import (
	"context"
	"time"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
	"zhilv-yuntu-go/internal/services"
)

type WeatherUsecase struct {
	service *services.WeatherService
}

func NewWeatherUsecase(service *services.WeatherService) *WeatherUsecase {
	return &WeatherUsecase{service: service}
}

func (u *WeatherUsecase) Forecast(ctx context.Context, city string) domain.WeatherForecastResponse {
	start := time.Now()
	logging.Info(ctx, "weather usecase forecast started", "city", city)
	forecast := u.service.Forecast(ctx, city)
	logging.Info(ctx, "weather usecase forecast completed",
		"city", city,
		"days", len(forecast.Days),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return forecast
}
