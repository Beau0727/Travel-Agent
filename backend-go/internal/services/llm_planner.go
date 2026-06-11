package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/llm"
	"travel-agent-go/internal/logging"
)

// LLMPlanner 是 Planner 的大模型实现。
// 这个文件有意不用第三方 SDK，而是用 net/http 手写一次 OpenAI-compatible 请求，
// 这样你可以看清楚 Go 调 HTTP、定义请求结构、解析 JSON 的完整过程。
type LLMPlanner struct {
	cfg    config.Config
	client llm.ChatClient
}

func NewLLMPlanner(cfg config.Config, client llm.ChatClient) *LLMPlanner {
	return &LLMPlanner{
		cfg:    cfg,
		client: client,
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
		"字段：summary, tips, days。tips 必须是字符串数组。days 必须恰好包含 " + itoa(dayCount) + " 项。\n" +
		"days 每项字段：day_index, theme, spot_name, spot_description, meal_name, meal_notes, daily_note。\n" +
		"spot_name 和 meal_name 必须是真实地点或餐饮名称，不要使用“推荐景点 1”“特色餐饮 1”等占位词。\n" +
		"优先使用上下文中的在线证据、候选景点、官方/地图/票务来源和本地攻略信息。\n" +
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

	var payload plannerJSONPayload
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &payload); err != nil {
		logging.Warn(ctx, "llm planner generate draft parse failed",
			"destination", request.Destination,
			"duration_ms", time.Since(start).Milliseconds(),
			"raw_chars", len([]rune(raw)),
			"error", err,
		)
		return PlannerDraft{}, false, err
	}
	if len(payload.Days) < dayCount {
		return PlannerDraft{}, false, errors.New("LLM 返回天数不匹配")
	}
	if len(payload.Days) > dayCount {
		payload.Days = payload.Days[:dayCount]
	}

	days := make([]PlannerDayDraft, 0, len(payload.Days))
	for _, item := range payload.Days {
		days = append(days, PlannerDayDraft{
			DayIndex:        int(item.DayIndex),
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
	history, _ := json.Marshal(firstNEditMessages(request.Messages, 8))
	constraints, _ := json.Marshal(request.PreserveConstraints)
	prompt := "你是一名旅行行程编辑助手。请只输出 JSON，不要 Markdown。\n" +
		"字段：theme, spot_name, spot_description, meal_name, meal_notes, daily_note, change_summary。\n" +
		"change_summary 是字符串数组，用 1 到 4 条中文短句说明本轮具体修改了什么。\n" +
		"当前目标日：" + string(currentDay) + "\n" +
		"用户编辑指令：" + request.UserInstruction + "\n" +
		"编辑范围：" + request.EditScope + "\n" +
		"必须保留的约束：" + string(constraints) + "\n" +
		"最近编辑对话历史：" + string(history)

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
	if p.client == nil {
		return "", errors.New("llm chat client is not configured")
	}

	logging.Info(ctx, "llm planner chat request started",
		"model", p.cfg.LLMModel,
		"prompt_chars", len([]rune(prompt)),
	)
	message, err := p.client.Chat(ctx, llm.ChatRequest{
		Model: p.cfg.LLMModel,
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "You are a strict JSON API. Return only one valid JSON object matching the requested schema."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
	})
	if err != nil {
		logging.Warn(ctx, "llm planner chat failed",
			"model", p.cfg.LLMModel,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", err
	}
	logging.Info(ctx, "llm planner chat request completed",
		"model", p.cfg.LLMModel,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return message.Content, nil
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

type plannerJSONPayload struct {
	Summary string             `json:"summary"`
	Tips    flexibleStringList `json:"tips"`
	Days    []plannerJSONDay   `json:"days"`
}

type plannerJSONDay struct {
	DayIndex        flexibleInt `json:"day_index"`
	Theme           string      `json:"theme"`
	SpotName        string      `json:"spot_name"`
	SpotDescription string      `json:"spot_description"`
	MealName        string      `json:"meal_name"`
	MealNotes       string      `json:"meal_notes"`
	DailyNote       string      `json:"daily_note"`
}

type flexibleStringList []string

func (l *flexibleStringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*l = nil
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*l = cleanStringList(list)
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			*l = nil
			return nil
		}
		*l = []string{strings.TrimSpace(single)}
		return nil
	}
	return errors.New("expected string or string array")
}

type flexibleInt int

func (i *flexibleInt) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*i = flexibleInt(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return err
	}
	*i = flexibleInt(parsed)
	return nil
}
