package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var videoTrimSlots = make(chan struct{}, 2)

func TrimVideo(ctx context.Context, input io.Reader, start, end float64) (string, func(), error) {
	trimDuration := math.Round((end-start)*1000) / 1000
	if math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || start < 0 || trimDuration < 3 || trimDuration > 14.9 {
		return "", nil, errors.New("请选择 3–14.9 秒的片段，为编码尾帧保留余量")
	}
	select {
	case videoTrimSlots <- struct{}{}:
		defer func() { <-videoTrimSlots }()
	default:
		return "", nil, errors.New("视频裁剪繁忙，请稍后重试")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil { return "", nil, errors.New("当前服务未安装 FFmpeg，请更新到包含视频裁剪的镜像") }
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil { return "", nil, errors.New("当前服务未安装 FFprobe，请更新镜像") }
	dir, err := os.MkdirTemp("", "canvas-video-trim-")
	if err != nil { return "", nil, errors.New("无法创建视频裁剪临时目录") }
	cleanup := func() { _ = os.RemoveAll(dir) }
	success := false
	defer func() { if !success { cleanup() } }()
	src := filepath.Join(dir, "source")
	f, err := os.Create(src)
	if err != nil { return "", nil, errors.New("无法保存原视频") }
	n, copyErr := io.Copy(f, io.LimitReader(input, (200<<20)+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n > 200<<20 { return "", nil, errors.New("原视频读取失败或超过 200MB") }
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	probe := func(path string) (float64, int, int, error) {
		data, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-select_streams", "v:0", "-show_entries", "stream=width,height:format=duration", "-of", "json", path).Output()
		if err != nil { return 0, 0, 0, errors.New("无法读取视频，请使用 MP4、MOV 或 WebM 文件") }
		var meta struct {
			Format struct { Duration string `json:"duration"` } `json:"format"`
			Streams []struct { Width int `json:"width"`; Height int `json:"height"` } `json:"streams"`
		}
		if json.Unmarshal(data, &meta) != nil || len(meta.Streams) == 0 { return 0, 0, 0, errors.New("文件没有可用的视频画面") }
		duration, err := strconv.ParseFloat(meta.Format.Duration, 64)
		if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 { return 0, 0, 0, errors.New("无法确定视频实际长度") }
		return duration, meta.Streams[0].Width, meta.Streams[0].Height, nil
	}
	duration, width, height, err := probe(src)
	if err != nil { return "", nil, err }
	if end > duration || width <= 0 || height <= 0 || width > 3840 || height > 3840 { return "", nil, errors.New("裁剪范围超过原片，或视频尺寸超过 3840 像素") }
	out := filepath.Join(dir, "trimmed.mp4")
	args := []string{"-v", "error", "-nostdin", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-ss", strconv.FormatFloat(start, 'f', 6, 64), "-i", src, "-t", strconv.FormatFloat(trimDuration, 'f', 6, 64), "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-threads", "2", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", "-y", out}
	if exec.CommandContext(ctx, ffmpeg, args...).Run() != nil { return "", nil, errors.New("视频裁剪失败或超时，请缩短片段后重试") }
	actual, _, _, err := probe(out)
	if err != nil || actual < 3 || actual > 15 { return "", nil, errors.New("裁剪结果仍不符合 3–15 秒限制，请将结束位置再提前 0.1 秒") }
	success = true
	return out, cleanup, nil
}
