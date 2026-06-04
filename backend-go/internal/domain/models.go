package domain

// 这个文件对应 Python 版的 backend/app/models/schemas.py。
// Go 没有 Pydantic，最常见的做法是用 struct 表达数据结构，
// 再用 `json:"field_name"` 标签声明 HTTP JSON 字段名。

// TripRequest 是“生成行程”的入参。
// 前端发送的字段名保持和 Python 版一致，方便 Vue 前端直接切换 API 地址。
type TripRequest struct {
	Destination        string   `json:"destination"`
	StartDate          string   `json:"start_date"`
	EndDate            string   `json:"end_date"`
	Travelers          int      `json:"travelers"`
	Budget             float64  `json:"budget"`
	Preferences        []string `json:"preferences"`
	Pace               string   `json:"pace"`
	DietaryPreferences []string `json:"dietary_preferences"`
	HotelLevel         string   `json:"hotel_level"`
	SpecialNotes       string   `json:"special_notes"`
}

// TripEditRequest 是“智能编辑行程”的入参。
type TripEditRequest struct {
	TripID              string    `json:"trip_id"`
	CurrentItinerary    Itinerary `json:"current_itinerary"`
	UserInstruction     string    `json:"user_instruction"`
	EditScope           string    `json:"edit_scope"`
	PreserveConstraints []string  `json:"preserve_constraints"`
}

// TripSaveRequest 是“保存行程”的入参。
type TripSaveRequest struct {
	TripID    string    `json:"trip_id"`
	Itinerary Itinerary `json:"itinerary"`
	UserID    string    `json:"user_id"`
}

type SpotItem struct {
	Name          string   `json:"name"`
	StartTime     string   `json:"start_time,omitempty"`
	EndTime       string   `json:"end_time,omitempty"`
	Description   string   `json:"description,omitempty"`
	EstimatedCost float64  `json:"estimated_cost"`
	Location      string   `json:"location,omitempty"`
	ImageURL      string   `json:"image_url,omitempty"`
	Address       string   `json:"address,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	POIID         string   `json:"poi_id,omitempty"`
}

type MealItem struct {
	Name          string   `json:"name"`
	MealType      string   `json:"meal_type"`
	EstimatedCost float64  `json:"estimated_cost"`
	Notes         string   `json:"notes,omitempty"`
	Location      string   `json:"location,omitempty"`
	Address       string   `json:"address,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	POIID         string   `json:"poi_id,omitempty"`
}

type HotelItem struct {
	Name          string   `json:"name"`
	Level         string   `json:"level,omitempty"`
	EstimatedCost float64  `json:"estimated_cost"`
	Location      string   `json:"location,omitempty"`
	Address       string   `json:"address,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
}

type TransportItem struct {
	Mode             string   `json:"mode"`
	FromPlace        string   `json:"from_place,omitempty"`
	ToPlace          string   `json:"to_place,omitempty"`
	EstimatedCost    float64  `json:"estimated_cost"`
	Duration         string   `json:"duration,omitempty"`
	DistanceKM       *float64 `json:"distance_km,omitempty"`
	EstimatedMinutes *int     `json:"estimated_minutes,omitempty"`
}

type BudgetBreakdown struct {
	Transport float64 `json:"transport"`
	Hotel     float64 `json:"hotel"`
	Meals     float64 `json:"meals"`
	Tickets   float64 `json:"tickets"`
	Other     float64 `json:"other"`
	Total     float64 `json:"total"`
}

type DayPlan struct {
	DayIndex  int             `json:"day_index"`
	Date      string          `json:"date,omitempty"`
	Theme     string          `json:"theme,omitempty"`
	Spots     []SpotItem      `json:"spots"`
	Meals     []MealItem      `json:"meals"`
	Hotel     *HotelItem      `json:"hotel,omitempty"`
	Transport []TransportItem `json:"transport"`
	Notes     []string        `json:"notes"`
}

type Itinerary struct {
	TripID          string          `json:"trip_id"`
	Destination     string          `json:"destination"`
	Summary         string          `json:"summary"`
	Days            []DayPlan       `json:"days"`
	EstimatedBudget float64         `json:"estimated_budget"`
	BudgetBreakdown BudgetBreakdown `json:"budget_breakdown"`
	Tips            []string        `json:"tips"`
	SourceNotes     []string        `json:"source_notes"`
}

type TripDetailResponse struct {
	TripID    string    `json:"trip_id"`
	Itinerary Itinerary `json:"itinerary"`
	CreatedAt string    `json:"created_at,omitempty"`
	UpdatedAt string    `json:"updated_at,omitempty"`
}

type TripSummaryItem struct {
	TripID      string `json:"trip_id"`
	Destination string `json:"destination"`
	Summary     string `json:"summary"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type TripListResponse struct {
	Total int               `json:"total"`
	Items []TripSummaryItem `json:"items"`
}

type WeatherForecastDay struct {
	Date         string `json:"date,omitempty"`
	Week         string `json:"week,omitempty"`
	DayWeather   string `json:"day_weather,omitempty"`
	NightWeather string `json:"night_weather,omitempty"`
	DayTemp      string `json:"day_temp,omitempty"`
	NightTemp    string `json:"night_temp,omitempty"`
	DayWind      string `json:"day_wind,omitempty"`
	NightWind    string `json:"night_wind,omitempty"`
}

type WeatherForecastResponse struct {
	City       string               `json:"city"`
	Province   string               `json:"province,omitempty"`
	Adcode     string               `json:"adcode,omitempty"`
	ReportTime string               `json:"report_time,omitempty"`
	Days       []WeatherForecastDay `json:"days"`
}
