package handler

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tigerowo/infinite-canvas/service"
)

func TrimVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 201<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if r.MultipartForm != nil { _ = r.MultipartForm.RemoveAll() }
		Fail(w, "视频上传失败，文件不能超过 200MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil { Fail(w, "请选择原视频"); return }
	defer file.Close()
	start, startErr := strconv.ParseFloat(r.FormValue("start"), 64)
	end, endErr := strconv.ParseFloat(r.FormValue("end"), 64)
	if startErr != nil || endErr != nil { Fail(w, "裁剪时间无效"); return }
	path, cleanup, err := service.TrimVideo(r.Context(), file, start, end)
	if err != nil { Fail(w, err.Error()); return }
	defer cleanup()
	output, err := os.Open(path)
	if err != nil { Fail(w, "无法读取裁剪结果"); return }
	defer output.Close()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "trimmed.mp4", time.Time{}, output)
}
