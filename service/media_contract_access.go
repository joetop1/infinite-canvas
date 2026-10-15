package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
)

// CheckMediaContractAccess checks reachability; it never changes storage or uploads files.
func CheckMediaContractAccess(ctx context.Context, channel model.ModelChannel, name string, body []byte) error {
	if !channel.StrictContracts || !usesMediaContract(channel) {
		return nil
	}
	entry := loadMediaContract(channel, name)
	if entry.Description.Status != "ready" {
		return fmt.Errorf("模型规范尚未就绪")
	}
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		return fmt.Errorf("素材参数无法解析")
	}
	if IsReplicateChannel(channel) {
		input, _ = input["input"].(map[string]any)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, role := range []string{"images", "videos", "audios", "first_frame", "last_frame"} {
		field, ok := entry.Description.Bindings[role]
		if !ok {
			continue
		}
		for _, value := range contractReferences(input[field.Name]) {
			if strings.HasPrefix(value, "data:") || seen[value] {
				continue
			}
			seen[value] = true
			u, err := url.Parse(value)
			if err != nil || u.Scheme != "https" && u.Scheme != "http" {
				return fmt.Errorf("%s不是可下载的网络地址，尚未提交模型", mediaRoleLabel(role))
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
			if err != nil {
				return fmt.Errorf("%s地址无效", mediaRoleLabel(role))
			}
			request.Header.Set("Range", "bytes=0-0")
			response, err := SafeProxyHTTPClient().Do(request)
			if err != nil {
				return fmt.Errorf("%s无法匿名读取，尚未提交模型；请检查存储直链", mediaRoleLabel(role))
			}
			status := response.StatusCode
			contentType := strings.ToLower(response.Header.Get("Content-Type"))
			n, readErr := io.CopyN(io.Discard, response.Body, 1)
			response.Body.Close()
			if status != http.StatusOK && status != http.StatusPartialContent || readErr != nil || n != 1 || strings.Contains(contentType, "text/html") {
				return fmt.Errorf("%s匿名读取失败（HTTP %d），尚未提交模型；请使用上游可读取的存储直链", mediaRoleLabel(role), status)
			}
		}
	}
	return nil
}
