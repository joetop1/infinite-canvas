package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

// StageSeedanceVideoMedia gives Seedance temporary read access to stored references.
func StageSeedanceVideoMedia(ctx context.Context, modelName string, siteHosts []string, body []byte) ([]byte, error) {
	if !strings.Contains(strings.ToLower(modelName), "seedance") {
		return body, nil
	}
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	payload := input
	if metadata, ok := input["metadata"].(map[string]any); ok {
		payload = metadata
	}
	content, ok := payload["content"].([]any)
	if !ok {
		return body, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	prepared := map[string]string{}
	for i, value := range content {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := item["type"].(string)
		if kind != "image_url" && kind != "video_url" && kind != "audio_url" {
			continue
		}
		media, ok := item[kind].(map[string]any)
		if !ok {
			continue
		}
		value, _ := media["url"].(string)
		u, err := url.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("参考素材 %d 地址无效", i+1)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "api" || parts[1] != "files" || parts[3] != "content" {
			continue
		}
		matchesSite := u.Host == "" && strings.HasPrefix(value, "/api/files/")
		for _, host := range siteHosts {
			for _, candidate := range strings.Split(host, ",") {
				matchesSite = matchesSite || strings.EqualFold(u.Host, strings.TrimSpace(candidate))
			}
		}
		if !matchesSite {
			if _, err := StorageObjectInfo(parts[2]); err == nil {
				return nil, fmt.Errorf("参考素材 %d 的本站域名无法识别，尚未提交模型", i+1)
			}
			continue
		}
		link := prepared[parts[2]]
		if link == "" {
			link, err = storedMediaReadURL(ctx, parts[2])
			if err != nil {
				return nil, fmt.Errorf("参考素材 %d 准备失败：%w；尚未提交模型", i+1, err)
			}
			prepared[parts[2]] = link
		}
		media["url"] = link
	}
	return json.Marshal(input)
}

func storedMediaReadURL(ctx context.Context, id string) (string, error) {
	object, err := repository.GetStorageObject(id)
	if err != nil || object.DeletedAt != "" {
		return "", fmt.Errorf("文件记录不存在或已删除")
	}
	if !strings.HasPrefix(object.MimeType, "image/") && !strings.HasPrefix(object.MimeType, "video/") && !strings.HasPrefix(object.MimeType, "audio/") {
		return "", fmt.Errorf("文件不是图片、视频或音频")
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return "", fmt.Errorf("无法读取存储配置")
	}
	providers := normalizePrivateStorageSetting(settings.Private.Storage).Providers
	if object.CreatedBy != "" && object.CreatedBy != "anonymous" {
		config, found, loadErr := repository.GetUserConfig(object.CreatedBy)
		if loadErr != nil {
			return "", fmt.Errorf("无法读取素材所属存储配置")
		}
		if found {
			providers = append(userStorageProvidersForOwner(config.StorageProvider, object.CreatedBy), providers...)
		}
	}
	provider, found := findStorageProviderForObject(object, providers)
	if !found || provider.Type != model.StorageProviderTypeS3 || !storageProviderConfigured(provider) {
		return "", fmt.Errorf("本站参考素材需要配置 S3/R2 存储以生成限时下载链接")
	}
	link, err := presignStoredMediaGET(provider, object.ObjectKey)
	if err != nil {
		return "", err
	}
	// Check the exact GET URL without downloading the entire video.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", fmt.Errorf("无法准备素材下载请求")
	}
	request.Header.Set("Range", "bytes=0-0")
	response, err := SafeProxyHTTPClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("存储直链暂时无法读取，请稍后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("存储直链返回 HTTP %d", response.StatusCode)
	}
	if n, err := io.CopyN(io.Discard, response.Body, 1); err != nil || n != 1 {
		return "", fmt.Errorf("存储直链未返回文件内容")
	}
	return link, nil
}

// Reuse the existing storage SigV4 primitives for a GET-only, 24-hour URL.
func presignStoredMediaGET(provider model.StorageProvider, objectKey string) (string, error) {
	u, err := url.Parse(strings.TrimRight(provider.Endpoint, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("S3/R2 存储须配置有效的 HTTPS 端点")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + provider.Bucket + "/" + strings.TrimLeft(objectKey, "/")
	segments := strings.Split(u.Path, "/")
	for i := range segments {
		segments[i] = strings.ReplaceAll(url.QueryEscape(segments[i]), "+", "%20")
	}
	u.RawPath = strings.Join(segments, "/")
	now := time.Now().UTC()
	date := now.Format("20060102")
	region := provider.Region
	if region == "" {
		region = "auto"
	}
	scope := date + "/" + region + "/s3/aws4_request"
	query := url.Values{
		"X-Amz-Algorithm":     {"AWS4-HMAC-SHA256"},
		"X-Amz-Credential":    {provider.AccessKeyID + "/" + scope},
		"X-Amz-Date":          {now.Format("20060102T150405Z")},
		"X-Amz-Expires":       {"86400"},
		"X-Amz-SignedHeaders": {"host"},
	}
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	canonical := "GET\n" + u.EscapedPath() + "\n" + u.RawQuery + "\nhost:" + u.Host + "\n\nhost\nUNSIGNED-PAYLOAD"
	toSign := "AWS4-HMAC-SHA256\n" + query.Get("X-Amz-Date") + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	query.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(signingKey(provider.SecretAccessKey, date, region), []byte(toSign))))
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return u.String(), nil
}
