export interface TripRequestPayload {
  destination: string;
  start_date: string;
  end_date: string;
  travelers: number;
  budget: number;
  preferences: string[];
  pace?: string | null;
  dietary_preferences: string[];
  hotel_level?: string | null;
  special_notes?: string | null;
}

export interface TripEditPayload {
  trip_id: string;
  current_itinerary: Itinerary;
  user_instruction: string;
  edit_scope?: string | null;
  preserve_constraints: string[];
  conversation_id?: string | null;
  messages?: TripEditMessage[];
}

export interface SpotItem {
  name: string;
  start_time?: string | null;
  end_time?: string | null;
  description?: string | null;
  estimated_cost?: number;
  location?: string | null;
  image_url?: string | null;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  poi_id?: string | null;
}

export interface MealItem {
  name: string;
  meal_type: string;
  estimated_cost?: number;
  notes?: string | null;
  location?: string | null;
  image_url?: string | null;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  poi_id?: string | null;
}

export interface HotelItem {
  name: string;
  level?: string | null;
  estimated_cost?: number;
  location?: string | null;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
}

export interface TransportItem {
  mode: string;
  from_place?: string | null;
  to_place?: string | null;
  estimated_cost?: number;
  duration?: string | null;
  distance_km?: number | null;
  estimated_minutes?: number | null;
  route_provider?: string | null;
  route_mode?: string | null;
  route_status?: string | null;
  distance_meters?: number;
  duration_seconds?: number;
  polyline?: string | null;
  route_summary?: string | null;
  route_warning?: string | null;
  route_taxi_cost?: number;
}

export interface DayPlan {
  day_index: number;
  date?: string | null;
  theme?: string | null;
  spots: SpotItem[];
  meals: MealItem[];
  hotel?: HotelItem | null;
  transport: TransportItem[];
  notes: string[];
}

export interface BudgetBreakdown {
  transport: number;
  hotel: number;
  meals: number;
  tickets: number;
  other: number;
  total: number;
}

export interface EvidenceSource {
  id: string;
  title?: string | null;
  url?: string | null;
  host?: string | null;
  source_type?: string | null;
  verification_role?: string | null;
  source_priority?: number | null;
  reliability_label?: string | null;
  reliability_score: number;
  published_at?: string | null;
  retrieved_at?: string | null;
  snippet?: string | null;
}

export interface EvidenceClaim {
  id: string;
  claim_type: string;
  name?: string | null;
  claim: string;
  status: string;
  confidence: number;
  source_ids?: string[];
  source_urls?: string[];
  source_types?: string[];
  requires_review: boolean;
  risk_level?: string | null;
  reason?: string | null;
  verification_status?: string | null;
  verification_channels?: string[];
  verification_summary?: string | null;
  official_source_url?: string | null;
}

export interface EvidenceReport {
  destination?: string | null;
  query?: string | null;
  generated_at?: string | null;
  summary?: string[];
  verification_summary?: string[];
  sources?: EvidenceSource[];
  claims?: EvidenceClaim[];
  warnings?: string[];
}

export interface Itinerary {
  trip_id: string;
  destination: string;
  summary: string;
  days: DayPlan[];
  estimated_budget: number;
  budget_breakdown: BudgetBreakdown;
  tips: string[];
  source_notes: string[];
  evidence?: EvidenceReport | null;
  edit_conversation_id?: string | null;
  edit_messages?: TripEditMessage[];
  edit_revisions?: TripEditRevision[];
  last_change_summary?: string[];
  edit_issues?: TripEditIssue[];
}

export interface TripEditMessage {
  role: "user" | "assistant" | "tool" | string;
  content: string;
  created_at?: string | null;
}

export interface TripEditRevision {
  revision_id: string;
  trip_id: string;
  instruction: string;
  edit_scope: string;
  change_summary: string[];
  created_at: string;
}

export interface TripEditIssue {
  code: string;
  level: string;
  message: string;
  day_index?: number;
}

export interface TripSaveResponse {
  message: string;
  trip_id: string;
}

export interface TripSummaryItem {
  trip_id: string;
  destination: string;
  summary: string;
  created_at?: string | null;
  updated_at?: string | null;
}

export interface TripListResponse {
  total: number;
  items: TripSummaryItem[];
}

export interface TripDetailResponse {
  trip_id: string;
  itinerary: Itinerary;
  created_at?: string | null;
  updated_at?: string | null;
}

export interface WeatherForecastDay {
  date?: string | null;
  week?: string | null;
  day_weather?: string | null;
  night_weather?: string | null;
  day_temp?: string | null;
  night_temp?: string | null;
  day_wind?: string | null;
  night_wind?: string | null;
  day_power?: string | null;
  night_power?: string | null;
}

export interface WeatherForecastResponse {
  city: string;
  province?: string | null;
  adcode?: string | null;
  report_time?: string | null;
  source?: string | null;
  risks?: string[];
  advice?: string[];
  days: WeatherForecastDay[];
}
