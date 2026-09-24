package service

import (
	"net/http"
	"sort"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
)

const (
	ModelChannelProtocolOpenAI    = "openai"
	ModelChannelProtocolGrok2API  = "grok2api"
	ModelChannelProtocolAPIMart   = "apimart"
	ModelChannelProtocolKIE       = "kie"
	ModelChannelProtocol88API     = "88api"
	ModelChannelProtocolAutoDL    = "autodl"
	ModelChannelProtocolArk       = "ark"
	ModelChannelProtocolFal       = "fal"
	ModelChannelProtocolReplicate = "replicate"
)

type modelProtocolAdapter struct {
	buildURL  func(model.ModelChannel, string) string
	setAuth   func(*http.Request, model.ModelChannel)
	models    func(model.ModelChannel) ([]string, error)
	testModel func(model.ModelChannel, string) (string, error)
}

type modelProtocolRule struct {
	id      string
	matches func(model.ModelChannel, string) bool
}

var modelProtocolRegistry map[string]modelProtocolAdapter
var modelProtocolIDs = []string{ModelChannelProtocolOpenAI, ModelChannelProtocolGemini, ModelChannelProtocolGrok2API, ModelChannelProtocolMiniMax, ModelChannelProtocolAPIMart, ModelChannelProtocolKIE, ModelChannelProtocolMiMo, ModelChannelProtocol88API, ModelChannelProtocolAutoDL, ModelChannelProtocolArk, ModelChannelProtocolFal, ModelChannelProtocolReplicate}

func init() {
	compatible := modelProtocolAdapter{
		buildURL: buildOpenAIModelChannelURL,
		setAuth: func(request *http.Request, channel model.ModelChannel) {
			request.Header.Set("Authorization", "Bearer "+channel.APIKey)
		},
		models:    fetchOpenAIAdminChannelModels,
		testModel: testOpenAIChannelModel,
	}
	modelProtocolRegistry = make(map[string]modelProtocolAdapter, 10)
	for _, id := range modelProtocolIDs {
		modelProtocolRegistry[id] = compatible
	}
	gemini := compatible
	gemini.buildURL = BuildGeminiChannelURL
	gemini.setAuth = func(request *http.Request, channel model.ModelChannel) {
		request.Header.Set("x-goog-api-key", channel.APIKey)
	}
	gemini.models = fetchGeminiAdminChannelModels
	gemini.testModel = testGeminiChannelModel
	modelProtocolRegistry[ModelChannelProtocolGemini] = gemini

	minimax := compatible
	minimax.buildURL = func(channel model.ModelChannel, path string) string {
		return normalizeModelChannelBaseURL(channel.BaseURL) + path
	}
	minimax.models = func(model.ModelChannel) ([]string, error) { return MiniMaxModels(), nil }
	minimax.testModel = func(model.ModelChannel, string) (string, error) {
		return "MiniMax-H3 是异步视频模型，请在视频创作台测试生成。", nil
	}
	modelProtocolRegistry[ModelChannelProtocolMiniMax] = minimax

	mimo := compatible
	mimo.models = func(model.ModelChannel) ([]string, error) {
		result := MiMoModels()
		sort.Strings(result)
		return result, nil
	}
	mimo.testModel = testMiMoTTSChannelModel
	modelProtocolRegistry[ModelChannelProtocolMiMo] = mimo

	kie := compatible
	kie.models = func(model.ModelChannel) ([]string, error) {
		result := kieMarketModels()
		sort.Strings(result)
		return result, nil
	}
	modelProtocolRegistry[ModelChannelProtocolKIE] = kie
	autodl := compatible
	autodl.buildURL = func(channel model.ModelChannel, path string) string {
		return BuildAutoDLURL(channel.BaseURL, path)
	}
	autodl.setAuth = func(request *http.Request, channel model.ModelChannel) {
		request.Header.Set("Authorization", channel.APIKey)
	}
	autodl.models = func(channel model.ModelChannel) ([]string, error) {
		workflows, err := AutoDLWorkflows(channel.BaseURL)
		ids := make([]string, 0, len(workflows))
		for _, workflow := range workflows {
			ids = append(ids, workflow.UUID)
		}
		return ids, err
	}
	autodl.testModel = func(channel model.ModelChannel, modelName string) (string, error) {
		if _, err := AutoDLWorkflowDetail(channel.BaseURL, modelName); err != nil {
			return "", err
		}
		return "AutoDL 工作流目录可读取；Token 和真实生成请在对应创作入口测试。", nil
	}
	modelProtocolRegistry[ModelChannelProtocolAutoDL] = autodl

	api88 := compatible
	api88.testModel = func(model.ModelChannel, string) (string, error) {
		return "88API 渠道不会调用聊天接口测试，请在对应创作台验证模型。", nil
	}
	modelProtocolRegistry[ModelChannelProtocol88API] = api88
	ark := compatible
	ark.testModel = testArkSeedanceChannelModel
	modelProtocolRegistry[ModelChannelProtocolArk] = ark

	// [CUSTOM] Fal.ai：提交与取结果都在队列域名下，且鉴权前缀是 `Key ` 而非 `Bearer `。
	fal := compatible
	fal.buildURL = func(channel model.ModelChannel, path string) string {
		return normalizeModelChannelBaseURL(channel.BaseURL) + path
	}
	fal.setAuth = func(request *http.Request, channel model.ModelChannel) {
		request.Header.Set("Authorization", FalAuthorizationHeader(channel.APIKey))
	}
	fal.models = func(model.ModelChannel) ([]string, error) {
		return FalModels(), nil
	}
	fal.testModel = func(model.ModelChannel, string) (string, error) {
		return "Fal.ai 模型请在图片或视频创作台发起一次生成验证。", nil
	}
	modelProtocolRegistry[ModelChannelProtocolFal] = fal

	// [CUSTOM] Replicate：官方模型与社区模型的创建地址不同，由 handler 层按 version 决定；
	// 地址沿用 OpenAI 的 /v1 归一化逻辑（默认 baseUrl 已带 /v1）。
	replicate := compatible
	replicate.models = func(model.ModelChannel) ([]string, error) {
		return ReplicateModels(), nil
	}
	replicate.testModel = func(model.ModelChannel, string) (string, error) {
		return "Replicate 模型请在图片或视频创作台发起一次生成验证。", nil
	}
	modelProtocolRegistry[ModelChannelProtocolReplicate] = replicate
	glm := compatible
	glm.testModel = testGLMTTSChannelModel
	modelProtocolRegistry["model:glm-tts"] = glm
}

// 发现模型、配置测试与生成的命中规则不同，分别保留原有优先级。
var modelDiscoveryRules = []modelProtocolRule{
	{ModelChannelProtocolAutoDL, func(channel model.ModelChannel, _ string) bool { return IsAutoDLChannel(channel) }},
	{ModelChannelProtocolGemini, func(channel model.ModelChannel, _ string) bool { return IsGeminiChannel(channel) }},
	{ModelChannelProtocolMiniMax, func(channel model.ModelChannel, _ string) bool { return IsMiniMaxChannel(channel) }},
	{ModelChannelProtocolMiMo, func(channel model.ModelChannel, _ string) bool { return IsMiMoChannel(channel) }},
	{ModelChannelProtocolArk, func(channel model.ModelChannel, _ string) bool { return IsArkChannel(channel) }},
	{ModelChannelProtocolKIE, func(channel model.ModelChannel, _ string) bool { return isKIEAdminChannel(channel) }},
	// [CUSTOM] 缺了这两条会让 Fal / Replicate 回落到 OpenAI 的 /models，
	// 打到 queue.fal.run/models 之类的地址上拿到 404。
	{ModelChannelProtocolFal, func(channel model.ModelChannel, _ string) bool { return IsFalChannel(channel) }},
	{ModelChannelProtocolReplicate, func(channel model.ModelChannel, _ string) bool { return IsReplicateChannel(channel) }},
}

var modelConfigTestRules = []modelProtocolRule{
	{ModelChannelProtocolAutoDL, func(channel model.ModelChannel, _ string) bool { return IsAutoDLChannel(channel) }},
	{ModelChannelProtocolMiniMax, func(channel model.ModelChannel, _ string) bool { return IsMiniMaxChannel(channel) }},
	{ModelChannelProtocol88API, func(channel model.ModelChannel, _ string) bool {
		return strings.EqualFold(strings.TrimSpace(channel.Protocol), ModelChannelProtocol88API)
	}},
	{ModelChannelProtocolArk, func(channel model.ModelChannel, _ string) bool { return IsArkChannel(channel) }},
	// [CUSTOM] 同上：不加这两条，「测试渠道」会拿 OpenAI 的 chat/completions 去测 Fal / Replicate。
	{ModelChannelProtocolFal, func(channel model.ModelChannel, _ string) bool { return IsFalChannel(channel) }},
	{ModelChannelProtocolReplicate, func(channel model.ModelChannel, _ string) bool { return IsReplicateChannel(channel) }},
}

var modelGenerationTestRules = []modelProtocolRule{
	{ModelChannelProtocolAutoDL, func(channel model.ModelChannel, _ string) bool { return IsAutoDLChannel(channel) }},
	{"model:glm-tts", func(_ model.ModelChannel, modelName string) bool {
		return strings.EqualFold(strings.TrimSpace(modelName), "glm-tts")
	}},
	{ModelChannelProtocolMiMo, func(_ model.ModelChannel, modelName string) bool { return IsMiMoTTSModelName(modelName) }},
	{ModelChannelProtocolGemini, func(channel model.ModelChannel, _ string) bool { return IsGeminiChannel(channel) }},
}

func modelProtocolForChannel(channel model.ModelChannel) modelProtocolAdapter {
	protocol := strings.TrimSpace(channel.Protocol)
	if adapter, ok := modelProtocolRegistry[protocol]; ok {
		return adapter
	}
	for _, id := range modelProtocolIDs {
		if strings.EqualFold(protocol, id) {
			return modelProtocolRegistry[id]
		}
	}
	return modelProtocolRegistry[ModelChannelProtocolOpenAI]
}

func IsArkChannel(channel model.ModelChannel) bool {
	return strings.EqualFold(strings.TrimSpace(channel.Protocol), ModelChannelProtocolArk)
}

func IsFalChannel(channel model.ModelChannel) bool {
	return strings.EqualFold(strings.TrimSpace(channel.Protocol), ModelChannelProtocolFal)
}

func IsReplicateChannel(channel model.ModelChannel) bool {
	return strings.EqualFold(strings.TrimSpace(channel.Protocol), ModelChannelProtocolReplicate)
}

// FalAuthorizationHeader 统一 Fal 的鉴权头前缀；用户已写成 `Key xxx` 时不再重复添加。
func FalAuthorizationHeader(apiKey string) string {
	key := strings.TrimSpace(apiKey)
	if strings.HasPrefix(strings.ToLower(key), "key ") || key == "" {
		return key
	}
	return "Key " + key
}

func matchModelProtocol(rules []modelProtocolRule, channel model.ModelChannel, modelName string) (modelProtocolAdapter, bool) {
	for _, rule := range rules {
		if rule.matches(channel, modelName) {
			return modelProtocolRegistry[rule.id], true
		}
	}
	return modelProtocolRegistry[ModelChannelProtocolOpenAI], false
}
