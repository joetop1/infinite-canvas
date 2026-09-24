package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
)

// [CUSTOM] Fal.ai 与 Replicate 的内置模型清单。
//
// 内置清单作为快速入口；Fal / Replicate 的完整公开目录通过搜索接口按关键词查询。
//
// 清单内的名称均已逐一验证存在：
//   - Fal 用 fal.ai/models/{endpoint_id} 页面探测，反向对照（故意写错的名字返回 404）确认有效；
//   - Replicate 同理用 replicate.com/{owner}/{name} 探测，luma/ray 等已失效项已剔除。
//
// 维护：模型下线时这里会失效，重新验证后替换即可。

// FalModels 返回 Fal.ai 的常用模型，元素是 endpoint id（提交地址为 {baseUrl}/{endpoint id}）。
// 顺序按用途分组，未做字典序排序——分组比字母序更便于在弹窗里定位。
func FalModels() []string {
	return []string{
		// 文生图
		"fal-ai/flux/dev",
		"fal-ai/flux/schnell",
		"fal-ai/flux-pro",
		"fal-ai/flux-pro/v1.1",
		"fal-ai/flux-pro/v1.1-ultra",
		"fal-ai/flux-lora",
		"fal-ai/fast-sdxl",
		"fal-ai/stable-diffusion-v35-large",
		"fal-ai/nano-banana-pro",
		"fal-ai/qwen-image",
		// 图生图 / 编辑
		"fal-ai/nano-banana-pro/edit",
		"fal-ai/nano-banana/edit",
		// 文生视频
		"fal-ai/veo3/fast",
		"fal-ai/kling-video/v2.1/master/text-to-video",
		"fal-ai/kling-video/v1.6/standard/text-to-video",
		"fal-ai/kling-video/v1.5/pro/text-to-video",
		"fal-ai/minimax/hailuo-02/standard/text-to-video",
		"fal-ai/minimax/video-01",
		"fal-ai/wan/v2.2-a14b/text-to-video",
		"fal-ai/bytedance/seedance/v1/pro/text-to-video",
		// 图生视频
		"fal-ai/kling-video/v1.5/pro/image-to-video",
		"fal-ai/minimax/video-01/image-to-video",
		"fal-ai/wan/v2.2-a14b/image-to-video",
		"fal-ai/bytedance/seedance/v1/pro/image-to-video",
	}
}

// ReplicateModels 返回 Replicate 的常用模型，元素是 owner/name。
//
// 这里以官方模型为主，因为官方模型可以直接 POST /v1/models/{owner}/{name}/predictions，
// 无需附带版本号；社区模型必须在名字后带 :version（或 ?version=hash），
// 版本号不便内置（会随模型更新失效），需要时由用户手动填写。
func ReplicateModels() []string {
	return []string{
		// 文生图
		"black-forest-labs/flux-schnell",
		"black-forest-labs/flux-dev",
		"black-forest-labs/flux-1.1-pro",
		"black-forest-labs/flux-2-pro",
		"black-forest-labs/flux-2-flex",
		"black-forest-labs/flux-kontext-pro",
		"black-forest-labs/flux-kontext-max",
		"google/imagen-4",
		"bytedance/seedream-3",
		"bytedance/seedream-4",
		"openai/gpt-image-1",
		"qwen/qwen-image-edit",
		"stability-ai/stable-diffusion-3.5-large",
		"recraft-ai/recraft-v3",
		"ideogram-ai/ideogram-v3-turbo",
		"prunaai/flux-fast",
		// 视频
		"google/veo-3-fast",
		"google/veo-3.1",
		"kwaivgi/kling-v2.1",
		"kwaivgi/kling-v2.5-turbo-pro",
		"minimax/video-01",
		"minimax/hailuo-02",
		"wan-video/wan-2.5-t2v",
		"wan-video/wan-2.5-i2v",
		"bytedance/seedance-1-pro",
	}
}

func searchFalModels(channel model.ModelChannel, query string) ([]string, error) {
	values := url.Values{"q": {query}, "limit": {"100"}, "status": {"active"}}
	request, err := http.NewRequest(http.MethodGet, "https://api.fal.ai/v1/models?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", FalAuthorizationHeader(channel.APIKey))
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return nil, safeMessageError{message: "搜索 Fal 模型失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, safeMessageError{message: "搜索 Fal 模型失败：无法读取上游响应"}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, readAdminChannelError(body, response.StatusCode, "搜索 Fal 模型失败")
	}
	var payload struct {
		Models []struct {
			EndpointID string `json:"endpoint_id"`
			ID         string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, safeMessageError{message: "搜索 Fal 模型失败：无法解析上游响应"}
	}
	models := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		if modelID := firstNonEmpty(strings.TrimSpace(item.EndpointID), strings.TrimSpace(item.ID)); modelID != "" {
			models = append(models, modelID)
		}
	}
	return uniqueSortedModels(models), nil
}

func searchReplicateModels(channel model.ModelChannel, query string) ([]string, error) {
	parsed, err := url.Parse(BuildModelChannelURL(channel, "/search"))
	if err != nil {
		return nil, err
	}
	values := parsed.Query()
	values.Set("query", query)
	values.Set("limit", "50")
	parsed.RawQuery = values.Encode()
	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	SetModelChannelAuthHeader(request, channel)
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return nil, safeMessageError{message: "搜索 Replicate 模型失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, safeMessageError{message: "搜索 Replicate 模型失败：无法读取上游响应"}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, readAdminChannelError(body, response.StatusCode, "搜索 Replicate 模型失败")
	}
	var payload struct {
		Results []json.RawMessage `json:"results"`
		Models  []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, safeMessageError{message: "搜索 Replicate 模型失败：无法解析上游响应"}
	}
	results := payload.Results
	if len(results) == 0 {
		results = payload.Models
	}
	models := make([]string, 0, len(results))
	for _, result := range results {
		if modelID := replicateSearchModelID(result); modelID != "" {
			models = append(models, modelID)
		}
	}
	return uniqueSortedModels(models), nil
}

func uniqueSortedModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, modelID := range models {
		if modelID == "" {
			continue
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		seen[modelID] = struct{}{}
		result = append(result, modelID)
	}
	sort.Strings(result)
	return result
}

func replicateSearchModelID(raw json.RawMessage) string {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil {
		return ""
	}
	var owner, name string
	_ = json.Unmarshal(item["owner"], &owner)
	_ = json.Unmarshal(item["name"], &name)
	if owner != "" && name != "" {
		modelID := owner + "/" + name
		var version struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(item["latest_version"], &version)
		if version.ID != "" {
			modelID += ":" + version.ID
		}
		return modelID
	}
	for _, key := range []string{"model", "result", "data"} {
		if nested := replicateSearchModelID(item[key]); nested != "" {
			return nested
		}
	}
	return ""
}
