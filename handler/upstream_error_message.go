package handler

import (
	"encoding/json"
	"strings"
	"unicode"
)

// [CUSTOM] 上游报错体的兼容层。
//
// 上游只认 OpenAI（error.message）/ new-api（msg）/ 通用（message）三种形状，
// 而 Fal.ai 与 Replicate 沿用 FastAPI 的 `{"detail": ...}`，读不出来时
// 用户只会看到一句「AI 接口请求失败：400」，等于没有信息。
// [CUSTOM] upstreamErrorTextLimit 限制带出的原文长度，避免把整页 HTML 或超大报文塞进用户可见报错。
const upstreamErrorTextLimit = 300

// readUpstreamDetailMessage 解析 FastAPI 风格的错误体：
// 字符串、`[{"msg":...,"loc":[...]}]` 列表（Fal / Replicate 的参数校验错误）两种形态。
func readUpstreamDetailMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var items []map[string]any
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	messages := make([]string, 0, len(items))
	for _, item := range items {
		message := firstNonEmpty(readUpstreamErrorText(item["msg"]), readUpstreamErrorText(item["message"]))
		if message == "" {
			// 校验错误常常只有 type + loc，例如 {"type":"missing","loc":["body","prompt"]}
			message = joinUpstreamErrorDetail(readUpstreamErrorText(item["type"]), readUpstreamErrorLoc(item["loc"]))
		}
		if message != "" {
			messages = append(messages, message)
		}
		if len(messages) == 3 {
			break
		}
	}
	return strings.Join(messages, "；")
}

func joinUpstreamErrorDetail(kind string, loc string) string {
	switch {
	case kind == "":
		return loc
	case loc == "":
		return kind
	default:
		return kind + ": " + loc
	}
}

func readUpstreamErrorLoc(value any) string {
	list, ok := value.([]any)
	if !ok {
		return readUpstreamErrorText(value)
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		if text := readUpstreamErrorText(item); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, ".")
}

func readUpstreamErrorText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64, bool:
		return toStringSafe(typed)
	default:
		return ""
	}
}

// readUpstreamPlainTextError 只在报文不是 JSON 时兜底，把上游原文带出来。
func readUpstreamPlainTextError(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" || len(text) > 4*upstreamErrorTextLimit {
		return ""
	}
	if strings.HasPrefix(text, "<") {
		return ""
	}
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" || strings.HasPrefix(cleaned, "<") {
		return ""
	}
	if runes := []rune(cleaned); len(runes) > upstreamErrorTextLimit {
		cleaned = string(runes[:upstreamErrorTextLimit]) + "…"
	}
	return cleaned
}
