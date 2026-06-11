package services

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	infraamap "travel-agent-go/internal/infrastructure/amap"
	"travel-agent-go/internal/logging"
)

type WeatherService struct {
	cfg      config.Config
	amap     *infraamap.Client
	resolver *infraamap.CityResolver
}

func NewWeatherService(cfg config.Config, amap *infraamap.Client) *WeatherService {
	return &WeatherService{
		cfg:      cfg,
		amap:     amap,
		resolver: infraamap.NewCityResolver(amap),
	}
}

func (s *WeatherService) Forecast(ctx context.Context, city string) domain.WeatherForecastResponse {
	if s == nil || !s.cfg.EnableAmapWeather || s.amap == nil || !s.amap.Enabled() {
		logging.Info(ctx, "weather service using demo forecast",
			"city", city,
			"reason", "amap weather disabled",
		)
		return demoWeatherForecast(city, "demo")
	}

	cityInfo, err := s.resolver.Resolve(ctx, city)
	if err != nil || cityInfo.Adcode == "" {
		logging.Warn(ctx, "weather city resolve failed, using demo forecast",
			"city", city,
			"error", err,
		)
		return demoWeatherForecast(city, "demo_fallback")
	}

	forecast, err := s.fetchAmapForecast(ctx, cityInfo)
	if err != nil {
		logging.Warn(ctx, "amap weather failed, using demo forecast",
			"city", city,
			"adcode", cityInfo.Adcode,
			"error", err,
		)
		return demoWeatherForecast(city, "demo_fallback")
	}

	logging.Info(ctx, "weather service forecast ready",
		"city", forecast.City,
		"source", forecast.Source,
		"days", len(forecast.Days),
	)
	return forecast
}

func (s *WeatherService) fetchAmapForecast(ctx context.Context, city infraamap.CityInfo) (domain.WeatherForecastResponse, error) {
	var raw struct {
		Status    string `json:"status"`
		Info      string `json:"info"`
		Infocode  string `json:"infocode"`
		Count     string `json:"count"`
		Forecasts []struct {
			City       string `json:"city"`
			Province   string `json:"province"`
			Adcode     string `json:"adcode"`
			ReportTime string `json:"reporttime"`
			Casts      []struct {
				Date         string `json:"date"`
				Week         string `json:"week"`
				DayWeather   string `json:"dayweather"`
				NightWeather string `json:"nightweather"`
				DayTemp      string `json:"daytemp"`
				NightTemp    string `json:"nighttemp"`
				DayWind      string `json:"daywind"`
				NightWind    string `json:"nightwind"`
				DayPower     string `json:"daypower"`
				NightPower   string `json:"nightpower"`
			} `json:"casts"`
		} `json:"forecasts"`
	}

	values := url.Values{}
	values.Set("city", city.Adcode)
	values.Set("extensions", "all")

	if err := s.amap.GetV3(ctx, "/weather/weatherInfo", values, &raw); err != nil {
		return domain.WeatherForecastResponse{}, err
	}
	if raw.Status != "1" || len(raw.Forecasts) == 0 {
		if raw.Info == "" {
			raw.Info = "empty weather forecast"
		}
		return domain.WeatherForecastResponse{}, errors.New(raw.Info)
	}

	item := raw.Forecasts[0]
	days := make([]domain.WeatherForecastDay, 0, len(item.Casts))
	for _, cast := range item.Casts {
		days = append(days, domain.WeatherForecastDay{
			Date:         cast.Date,
			Week:         cast.Week,
			DayWeather:   cast.DayWeather,
			NightWeather: cast.NightWeather,
			DayTemp:      cast.DayTemp,
			NightTemp:    cast.NightTemp,
			DayWind:      cast.DayWind,
			NightWind:    cast.NightWind,
			DayPower:     cast.DayPower,
			NightPower:   cast.NightPower,
		})
	}

	risks, advice := analyzeWeather(days)
	return domain.WeatherForecastResponse{
		City:       firstNonEmptyString(item.City, city.Name),
		Province:   firstNonEmptyString(item.Province, city.Province),
		Adcode:     firstNonEmptyString(item.Adcode, city.Adcode),
		ReportTime: item.ReportTime,
		Source:     "amap",
		Risks:      risks,
		Advice:     advice,
		Days:       days,
	}, nil
}

func analyzeWeather(days []domain.WeatherForecastDay) ([]string, []string) {
	risks := []string{}
	advice := []string{}
	for _, day := range days {
		weatherText := day.DayWeather + day.NightWeather
		if containsAnyText(weatherText, []string{"雨", "雪", "雷", "暴"}) && !containsString(risks, "rain") {
			risks = append(risks, "rain")
			advice = append(advice, "预报包含雨雪或雷雨天气，建议减少长时间户外步行，并准备雨具。")
		}
		if maxTemp(day.DayTemp, day.NightTemp) >= 32 && !containsString(risks, "heat") {
			risks = append(risks, "heat")
			advice = append(advice, "气温较高，建议避开正午户外暴晒，补充饮水并安排室内休息点。")
		}
		if minTemp(day.DayTemp, day.NightTemp) <= 8 && !containsString(risks, "cold") {
			risks = append(risks, "cold")
			advice = append(advice, "早晚温度偏低，建议增加保暖衣物。")
		}
		if containsAnyText(day.DayPower+day.NightPower, []string{"5", "6", "7", "8"}) && !containsString(risks, "wind") {
			risks = append(risks, "wind")
			advice = append(advice, "风力偏大，临水、高处和户外拍摄行程要预留弹性。")
		}
	}
	return risks, advice
}

func demoWeatherForecast(city, source string) domain.WeatherForecastResponse {
	return domain.WeatherForecastResponse{
		City:       city,
		ReportTime: source,
		Source:     source,
		Risks:      []string{"demo"},
		Advice:     []string{"当前使用示例天气数据，真实出行前请再次确认天气。"},
		Days: []domain.WeatherForecastDay{
			{Date: "", Week: "1", DayWeather: "多云", NightWeather: "多云", DayTemp: "24", NightTemp: "16"},
			{Date: "", Week: "2", DayWeather: "晴", NightWeather: "多云", DayTemp: "25", NightTemp: "15"},
			{Date: "", Week: "3", DayWeather: "小雨", NightWeather: "阴", DayTemp: "22", NightTemp: "14"},
		},
	}
}

func containsAnyText(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func maxTemp(a, b string) int {
	ai, _ := strconv.Atoi(strings.TrimSpace(a))
	bi, _ := strconv.Atoi(strings.TrimSpace(b))
	if ai > bi {
		return ai
	}
	return bi
}

func minTemp(a, b string) int {
	ai, _ := strconv.Atoi(strings.TrimSpace(a))
	bi, _ := strconv.Atoi(strings.TrimSpace(b))
	if ai < bi {
		return ai
	}
	return bi
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
