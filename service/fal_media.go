package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
)

// StageFalEditMedia transfers authorized edit and H3 inputs to Fal before inference.
func StageFalEditMedia(ctx context.Context, channel model.ModelChannel, modelName string, siteHosts []string, body []byte) ([]byte, error) {
	name := strings.SplitN(modelName, "?", 2)[0]
	edit := name == "fal-ai/kling-video/o3/pro/video-to-video/edit" || name == "fal-ai/kling-video/o3/standard/video-to-video/edit"
	h3 := name == "minimax/h3-max/reference-to-video"
	if !IsFalChannel(channel) || (!edit && !h3) {
		return body, nil
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stage := func(value string) (string, error) {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.RawQuery != "" {
			return value, nil
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "api" || parts[1] != "files" || parts[3] != "content" {
			return value, nil
		}
		matchesSite := false
		for _, host := range siteHosts {
			for _, candidate := range strings.Split(host, ",") {
				matchesSite = matchesSite || strings.EqualFold(u.Host, strings.TrimSpace(candidate))
			}
		}
		if !matchesSite {
			if _, err := StorageObjectInfo(parts[2]); err == nil {
				return "", fmt.Errorf("本站素材域名识别失败，尚未提交模型，请刷新网页后重试")
			}
			return value, nil
		}
		upstream, err := url.Parse(channel.BaseURL)
		if err != nil || upstream.Scheme != "https" || (upstream.Hostname() != "queue.fal.run" && upstream.Hostname() != "fal.run") {
			return "", fmt.Errorf("本站素材转存需要 Fal 官方 HTTPS 渠道，请为自定义网关配置可公开访问的存储直链")
		}
		return uploadStoredFalMedia(ctx, channel, parts[2])
	}
	fields := []struct {
		name  string
		array bool
	}{{"video_url", false}, {"image_urls", true}}
	if h3 {
		fields = []struct {
			name  string
			array bool
		}{{"reference_image_urls", true}, {"reference_video_urls", true}, {"image_url", false}, {"end_image_url", false}}
	}
	staged := map[string]string{}
	for _, field := range fields {
		raw, exists := input[field.name]
		if !exists {
			continue
		}
		var values []string
		if field.array {
			if err := json.Unmarshal(raw, &values); err != nil {
				return nil, fmt.Errorf("%s 素材格式无效", field.name)
			}
		} else {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("%s 素材格式无效", field.name)
			}
			values = []string{value}
		}
		for i, value := range values {
			if value == "" {
				continue
			}
			if cached, ok := staged[value]; ok {
				values[i] = cached
				continue
			}
			result, err := stage(value)
			if err != nil {
				return nil, fmt.Errorf("%s 第 %d 份素材转存失败：%w", field.name, i+1, err)
			}
			staged[value], values[i] = result, result
		}
		if field.array {
			input[field.name], _ = json.Marshal(values)
		} else {
			input[field.name], _ = json.Marshal(values[0])
		}
	}
	return json.Marshal(input)
}

func uploadStoredFalMedia(ctx context.Context, channel model.ModelChannel, id string) (string, error) {
	download, err := DownloadStorageObject(id, "")
	if err != nil {
		return "", err
	}
	defer download.Stream.Close()
	const limit = 200 << 20
	if download.ContentLength > limit || (!strings.HasPrefix(download.Object.MimeType, "image/") && !strings.HasPrefix(download.Object.MimeType, "video/")) {
		return "", fmt.Errorf("转存素材须为不超过 200MB 的图片或视频")
	}
	file, err := os.CreateTemp("", "fal-edit-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	size, err := io.Copy(file, io.LimitReader(download.Stream, limit+1))
	if err != nil {
		return "", err
	}
	if size == 0 || size > limit {
		return "", fmt.Errorf("转存素材为空或超过 200MB")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	metadata, _ := json.Marshal(map[string]string{"file_name": path.Base(download.Object.ObjectKey), "content_type": download.Object.MimeType})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.fal.ai/storage/upload/initiate?storage_type=fal-cdn-v3", bytes.NewReader(metadata))
	if err != nil {
		return "", err
	}
	SetModelChannelAuthHeader(request, channel)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Fal-Object-Lifecycle", `{"expiration_duration_seconds":86400}`)
	response, err := SafeProxyHTTPClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("Fal 素材存储连接失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Fal 素材存储初始化失败：HTTP %d", response.StatusCode)
	}
	var result struct {
		UploadURL string `json:"upload_url"`
		FileURL   string `json:"file_url"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return "", fmt.Errorf("Fal 素材存储响应无效")
	}
	for _, value := range []string{result.UploadURL, result.FileURL} {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return "", fmt.Errorf("Fal 素材存储地址无效")
		}
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodPut, result.UploadURL, file)
	if err != nil {
		return "", err
	}
	request.ContentLength = size
	request.Header.Set("Content-Type", download.Object.MimeType)
	response, err = SafeProxyHTTPClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("上传 Fal 素材失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("上传 Fal 素材失败：HTTP %d", response.StatusCode)
	}
	return result.FileURL, nil
}
