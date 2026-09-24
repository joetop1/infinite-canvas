import { collectHTTPURLs, firstHTTPURL, firstString, normalizeDirectStatus, readDirectError, readPath, readString, uniqueHTTPURLs } from "./shared";
import type { DirectProtocolAdapter } from "./types";

// Replicate 预测协议：POST {base}/models/{owner}/{name}/predictions 提交，得到 prediction id；
// 轮询 GET {base}/predictions/{id}，status 为 starting/processing/succeeded/failed/canceled，
// 产物在 output（字符串 URL、URL 数组或含 url 的对象）。鉴权头是 Bearer。
export const replicateDirectProtocol: DirectProtocolAdapter = {
    pollPath: (taskId) => `/predictions/${encodeURIComponent(taskId)}`,
    readTaskId: (payload) => firstString(readPath(payload, "id"), readPath(payload, "prediction_id")),
    readCreatedVideoStatus: (payload) => normalizeDirectStatus(readString(readPath(payload, "status"))),
    readError: readReplicateError,
    readImagePoll(payload) {
        const status = normalizeDirectStatus(readString(readPath(payload, "status")));
        const urls = status === "completed" ? uniqueHTTPURLs(collectHTTPURLs(readPath(payload, "output"))) : [];
        return {
            urls,
            done: status === "completed",
            error: status === "failed" ? readReplicateError(payload) || "Replicate 图片生成失败" : "",
        };
    },
    readVideoPoll(payload, pollId, model) {
        const status = normalizeDirectStatus(readString(readPath(payload, "status")));
        const videoUrl = status === "completed" ? firstHTTPURL(readPath(payload, "output")) : "";
        return {
            id: firstString(readPath(payload, "id"), pollId),
            task_id: firstString(readPath(payload, "id"), pollId),
            status: videoUrl ? "completed" : status,
            ...(videoUrl ? { video_url: videoUrl, url: videoUrl } : {}),
            ...(status === "failed" ? { error: { message: readReplicateError(payload) || "Replicate 视频生成失败" } } : {}),
            model,
        };
    },
};

function readReplicateError(payload: unknown) {
    const detail = readPath(payload, "detail");
    if (typeof detail === "string" && detail.trim()) return detail.trim();
    const error = firstString(readPath(payload, "error"), readPath(payload, "error.message"), readPath(payload, "message"));
    if (error) return error;
    return readDirectError(payload);
}
