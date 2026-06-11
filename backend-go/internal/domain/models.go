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
	TripID              string            `json:"trip_id"`
	CurrentItinerary    Itinerary         `json:"current_itinerary"`
	UserInstruction     string            `json:"user_instruction"`
	EditScope           string            `json:"edit_scope"`
	PreserveConstraints []string          `json:"preserve_constraints"`
	ConversationID      string            `json:"conversation_id,omitempty"`
	Messages            []TripEditMessage `json:"messages,omitempty"`
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
	City          string   `json:"city,omitempty"`
	Adcode        string   `json:"adcode,omitempty"`
	ImageURL      string   `json:"image_url,omitempty"`
	Address       string   `json:"address,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	POIID         string   `json:"poi_id,omitempty"`
}

type MealItem struct {
	Name          string   `json:"name"`
	MealType      string   `json:"meal_type"`
	Time          string   `json:"time,omitempty"`
	EstimatedCost float64  `json:"estimated_cost"`
	Notes         string   `json:"notes,omitempty"`
	Location      string   `json:"location,omitempty"`
	City          string   `json:"city,omitempty"`
	Adcode        string   `json:"adcode,omitempty"`
	ImageURL      string   `json:"image_url,omitempty"`
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
	City          string   `json:"city,omitempty"`
	Adcode        string   `json:"adcode,omitempty"`
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

	RouteProvider   string  `json:"route_provider,omitempty"` // amap
	RouteMode       string  `json:"route_mode,omitempty"`     // walking/driving/transit
	RouteStatus     string  `json:"route_status,omitempty"`
	DistanceMeters  int     `json:"distance_meters,omitempty"`
	DurationSeconds int     `json:"duration_seconds,omitempty"`
	Polyline        string  `json:"polyline,omitempty"`
	RouteSummary    string  `json:"route_summary,omitempty"`
	RouteWarning    string  `json:"route_warning,omitempty"`
	RouteTaxiCost   float64 `json:"route_taxi_cost,omitempty"`
}

type EvidenceSource struct {
	ID               string  `json:"id"`
	Title            string  `json:"title,omitempty"`
	URL              string  `json:"url,omitempty"`
	Host             string  `json:"host,omitempty"`
	SourceType       string  `json:"source_type,omitempty"`
	VerificationRole string  `json:"verification_role,omitempty"`
	SourcePriority   int     `json:"source_priority,omitempty"`
	ReliabilityLabel string  `json:"reliability_label,omitempty"`
	ReliabilityScore float64 `json:"reliability_score"`
	PublishedAt      string  `json:"published_at,omitempty"`
	RetrievedAt      string  `json:"retrieved_at,omitempty"`
	Snippet          string  `json:"snippet,omitempty"`
}

type EvidenceClaim struct {
	ID                   string   `json:"id"`
	ClaimType            string   `json:"claim_type"`
	Name                 string   `json:"name,omitempty"`
	Claim                string   `json:"claim"`
	Status               string   `json:"status"`
	Confidence           float64  `json:"confidence"`
	SourceIDs            []string `json:"source_ids,omitempty"`
	SourceURLs           []string `json:"source_urls,omitempty"`
	SourceTypes          []string `json:"source_types,omitempty"`
	RequiresReview       bool     `json:"requires_review"`
	RiskLevel            string   `json:"risk_level,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	VerificationStatus   string   `json:"verification_status,omitempty"`
	VerificationChannels []string `json:"verification_channels,omitempty"`
	VerificationSummary  string   `json:"verification_summary,omitempty"`
	OfficialSourceURL    string   `json:"official_source_url,omitempty"`
}

type EvidenceReport struct {
	Destination         string           `json:"destination,omitempty"`
	Query               string           `json:"query,omitempty"`
	GeneratedAt         string           `json:"generated_at,omitempty"`
	Summary             []string         `json:"summary,omitempty"`
	VerificationSummary []string         `json:"verification_summary,omitempty"`
	Sources             []EvidenceSource `json:"sources,omitempty"`
	Claims              []EvidenceClaim  `json:"claims,omitempty"`
	Warnings            []string         `json:"warnings,omitempty"`
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
	TripID             string             `json:"trip_id"`
	Destination        string             `json:"destination"`
	Summary            string             `json:"summary"`
	Days               []DayPlan          `json:"days"`
	EstimatedBudget    float64            `json:"estimated_budget"`
	BudgetBreakdown    BudgetBreakdown    `json:"budget_breakdown"`
	Tips               []string           `json:"tips"`
	SourceNotes        []string           `json:"source_notes"`
	Evidence           *EvidenceReport    `json:"evidence,omitempty"`
	EditConversationID string             `json:"edit_conversation_id,omitempty"`
	EditMessages       []TripEditMessage  `json:"edit_messages,omitempty"`
	EditRevisions      []TripEditRevision `json:"edit_revisions,omitempty"`
	LastChangeSummary  []string           `json:"last_change_summary,omitempty"`
	EditIssues         []TripEditIssue    `json:"edit_issues,omitempty"`
}

type TripEditMessage struct {
	Role      string `json:"role"` // user / assistant / tool
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}

type TripEditRevision struct {
	RevisionID    string   `json:"revision_id"`
	TripID        string   `json:"trip_id"`
	Instruction   string   `json:"instruction"`
	EditScope     string   `json:"edit_scope"`
	ChangeSummary []string `json:"change_summary"`
	CreatedAt     string   `json:"created_at"`
}

type TripEditIssue struct {
	Code     string `json:"code"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	DayIndex int    `json:"day_index,omitempty"`
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
	DayPower     string `json:"day_power,omitempty"`
	NightPower   string `json:"night_power,omitempty"`
}

type WeatherForecastResponse struct {
	City       string               `json:"city"`
	Province   string               `json:"province,omitempty"`
	Adcode     string               `json:"adcode,omitempty"`
	ReportTime string               `json:"report_time,omitempty"`
	Source     string               `json:"source,omitempty"`
	Risks      []string             `json:"risks,omitempty"`
	Advice     []string             `json:"advice,omitempty"`
	Days       []WeatherForecastDay `json:"days"`
}
