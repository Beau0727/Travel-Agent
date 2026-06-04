package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/config"
	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// LLMPlanner 是 Planner 的大模型实现。
// 这个文件有意不用第三方 SDK，而是用 net/http 手写一次 OpenAI-compatible 请求，
// 这样你可以看清楚 Go 调 HTTP、定义请求结构、解析 JSON 的完整过程。
type LLMPlanner struct {
	cfg    config.Config
	client *http.Client
}

func NewLLMPlanner(cfg config.Config) *LLMPlanner {
	return &LLMPlanner{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.LLMTimeoutSeconds) * time.Second,
		},
	}
}

func (p *LLMPlanner) GenerateDraft(request domain.TripRequest, contexts []string, dayCount int) (PlannerDraft, bool, error) {
	ctx := context.Background()
	start := time.Now()
	if p.cfg.LLMAPIKey == "" {
		logging.Info(ctx, "llm planner generate skipped missing api key",
			"destination", request.Destination,
		)
		return PlannerDraft{}, false, nil
	}
	logging.Info(ctx, "llm planner generate draft started",
		"destination", request.Destination,
		"day_count", dayCount,
		"contexts", len(contexts),
		"model", p.cfg.LLMModel,
	)

	prompt := "你是一名旅行规划助手。请只输出 JSON，不要 Markdown。\n" +
		"字段：summary, tips, days。days 每项字段：day_index, theme, spot_name, spot_description, meal_name, meal_notes, daily_note。\n" +
		"目的地：" + request.Destination + "\n" +
		"日期：" + request.StartDate + " 至 " + request.EndDate + "\n" +
		"天数：" + itoa(dayCount) + "\n" +
		"人数：" + itoa(request.Travelers) + "\n" +
		"预算：" + floatToString(request.Budget) + "\n" +
		"偏好：" + strings.Join(request.Preferences, "、") + "\n" +
		"节奏：" + defaultString(request.Pace, "适中") + "\n" +
		"饮食偏好：" + strings.Join(request.DietaryPreferences, "、") + "\n" +
		"额外备注：" + request.SpecialNotes + "\n" +
		"本地攻略上下文：\n" + strings.Join(contexts, "\n\n")

	raw, err := p.chat(prompt)
	if err != nil {
		logging.Warn(ctx, "llm planner generate draft chat failed",
			"destination", request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return PlannerDraft{}, false, err
	}

	var payload struct {
		Summary string   `json:"summary"`
		Tips    []string `json:"tips"`
		Days    []struct {
			DayIndex        int    `json:"day_index"`
			Theme           string `json:"theme"`
			SpotName        string `json:"spot_name"`
			SpotDescription string `json:"spot_description"`
			MealName        string `json:"meal_name"`
			MealNotes       string `json:"meal_notes"`
			DailyNote       string `json:"daily_note"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &payload); err != nil {
		return PlannerDraft{}, false, err
	}
	if len(payload.Days) != dayCount {
		return PlannerDraft{}, false, errors.New("LLM 返回天数不匹配")
	}

	days := make([]PlannerDayDraft, 0, len(payload.Days))
	for _, item := range payload.Days {
		days = append(days, PlannerDayDraft{
			DayIndex:        item.DayIndex,
			Theme:           item.Theme,
			SpotName:        item.SpotName,
			SpotDescription: item.SpotDescription,
			MealName:        item.MealName,
			MealNotes:       item.MealNotes,
			DailyNote:       item.DailyNote,
		})
	}
	logging.Info(ctx, "llm planner generate draft completed",
		"destination", request.Destination,
		"days", len(days),
		"tips", len(payload.Tips),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return PlannerDraft{Summary: payload.Summary, Tips: payload.Tips, Days: days}, true, nil
}

func (p *LLMPlanner) EditDay(request domain.TripEditRequest, targetDay domain.DayPlan) (DayEditDraft, bool, error) {
	ctx := context.Background()
	start := time.Now()
	if p.cfg.LLMAPIKey == "" {
		logging.Info(ctx, "llm planner edit skipped missing api key",
			"trip_id", request.TripID,
			"target_day", targetDay.DayIndex,
		)
		return DayEditDraft{}, false, nil
	}
	logging.Info(ctx, "llm planner edit day started",
		"trip_id", request.TripID,
		"edit_scope", request.EditScope,
		"target_day", targetDay.DayIndex,
		"model", p.cfg.LLMModel,
	)

	currentDay, _ := json.Marshal(targetDay)
	prompt := "你是一名旅行行程编辑助手。请只输出 JSON，不要 Markdown。\n" +
		"字段：theme, spot_name, spot_description, meal_name, meal_notes, daily_note。\n" +
		"当前目标日：" + string(currentDay) + "\n" +
		"用户编辑指令：" + request.UserInstruction + "\n" +
		"编辑范围：" + request.EditScope

	raw, err := p.chat(prompt)
	if err != nil {
		logging.Warn(ctx, "llm planner edit day chat failed",
			"trip_id", request.TripID,
			"target_day", targetDay.DayIndex,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return DayEditDraft{}, false, err
	}
	var payload DayEditDraft
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &payload); err != nil {
		logging.Warn(ctx, "llm planner edit day parse failed",
			"trip_id", request.TripID,
			"target_day", targetDay.DayIndex,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return DayEditDraft{}, false, err
	}
	logging.Info(ctx, "llm planner edit day completed",
		"trip_id", request.TripID,
		"target_day", targetDay.DayIndex,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return payload, true, nil
}

func (p *LLMPlanner) chat(prompt string) (string, error) {
	ctx := context.Background()
	start := time.Now()
	baseURL := strings.TrimRight(p.cfg.LLMBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	body := map[string]any{
		"model": p.cfg.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你必须只输出一个 JSON 对象。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.LLMAPIKey)
	req.Header.Set("Content-Type", "application/json")

	logging.Info(ctx, "llm planner chat request started",
		"model", p.cfg.LLMModel,
		"url", baseURL+"/chat/completions",
		"prompt_chars", len([]rune(prompt)),
	)
	resp, err := p.client.Do(req)
	if err != nil {
		logging.Warn(ctx, "llm planner chat request failed",
			"model", p.cfg.LLMModel,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Warn(ctx, "llm planner chat response failed",
			"model", p.cfg.LLMModel,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", string(respBody),
		)
		return "", errors.New(string(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("LLM 响应没有 choices")
	}
	logging.Info(ctx, "llm planner chat request completed",
		"model", p.cfg.LLMModel,
		"status", resp.StatusCode,
		"choices", len(parsed.Choices),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return parsed.Choices[0].Message.Content, nil
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return text
	}
	return text[start : end+1]
}
