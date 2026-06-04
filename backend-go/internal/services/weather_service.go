package services

import (
	"context"

	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// WeatherService 是天气服务的教学版实现。
// Python 版会调用高德天气；Go 版先返回稳定的示例数据，方便你本地无 Key 时也能跑通闭环。
type WeatherService struct{}

func NewWeatherService() *WeatherService {
	return &WeatherService{}
}

func (s *WeatherService) Forecast(ctx context.Context, city string) domain.WeatherForecastResponse {
	logging.Info(ctx, "weather service using demo forecast", "city", city)
	forecast := domain.WeatherForecastResponse{
		City:       city,
		ReportTime: "demo",
		Days: []domain.WeatherForecastDay{
			{Date: "", Week: "1", DayWeather: "多云", NightWeather: "多云", DayTemp: "24", NightTemp: "16"},
			{Date: "", Week: "2", DayWeather: "晴", NightWeather: "多云", DayTemp: "25", NightTemp: "15"},
			{Date: "", Week: "3", DayWeather: "小雨", NightWeather: "阴", DayTemp: "22", NightTemp: "14"},
		},
	}
	logging.Info(ctx, "weather service forecast ready", "city", city, "days", len(forecast.Days))
	return forecast
}
