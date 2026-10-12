package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
)

type ChannelModelCatalog struct {
	Models []string `json:"models"`
	ModelCapabilities map[string]string `json:"modelCapabilities"`
	ModelMetadata map[string]json.RawMessage `json:"modelMetadata"`
}

func IsOpenRouterChannel(channel model.ModelChannel) bool {
	if strings.EqualFold(strings.TrimSpace(channel.Protocol), "openrouter") { return true }
	base, err := url.Parse(channel.BaseURL)
	return err == nil && strings.EqualFold(base.Hostname(), "openrouter.ai") && (channel.Protocol == "" || strings.EqualFold(channel.Protocol, "openai"))
}

func AdminOpenRouterCatalog(index *int, channel model.ModelChannel) (ChannelModelCatalog, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil { return ChannelModelCatalog{}, err }
	return FetchOpenRouterCatalog(resolved)
}

func FetchOpenRouterCatalog(channel model.ModelChannel) (ChannelModelCatalog, error) {
	result := ChannelModelCatalog{ModelCapabilities: map[string]string{}, ModelMetadata: map[string]json.RawMessage{}}
	for _, endpoint := range []struct{ path, kind string }{{"/models", ""}, {"/images/models", "image"}, {"/videos/models", "video"}} {
		request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, endpoint.path), nil)
		if err != nil { return result, err }
		SetModelChannelAuthHeader(request, channel)
		response, err := adminModelHTTPClient.Do(request)
		if err != nil { return result, fmt.Errorf("OpenRouter 模型目录连接失败：%w", err) }
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil { return result, fmt.Errorf("OpenRouter 模型目录读取不完整：%w", readErr) }
		if response.StatusCode >= 400 { return result, readAdminChannelError(body, response.StatusCode, "OpenRouter 模型目录读取失败") }
		var payload struct { Data []json.RawMessage `json:"data"` }
		if err := json.Unmarshal(body, &payload); err != nil { return result, errors.New("OpenRouter 模型目录返回格式异常") }
		for _, raw := range payload.Data {
			var item struct { ID string `json:"id"`; Architecture struct { Outputs []string `json:"output_modalities"` } `json:"architecture"` }
			if json.Unmarshal(raw, &item) != nil || item.ID == "" { continue }
			kind := endpoint.kind
			if kind == "" {
				for _, output := range item.Architecture.Outputs {
					if output == "text" { kind = "text" }
					if output == "image" || output == "video" { kind = output; break }
				}
			}
			if kind == "" { continue }
			result.ModelCapabilities[item.ID] = kind
			result.ModelMetadata[item.ID] = raw
		}
	}
	for id := range result.ModelCapabilities { result.Models = append(result.Models, id) }
	sort.Strings(result.Models)
	return result, nil
}

var openRouterOmniPattern = regexp.MustCompile(`(?i)^bytedance(?:-seed)?/seedance-2(?:\.0)?(?:$|-)`)

func OpenRouterOmniModel(name string) bool { return openRouterOmniPattern.MatchString(name) }
