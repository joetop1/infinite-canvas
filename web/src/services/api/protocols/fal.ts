import { asRecord, firstHTTPURL, firstString, normalizeDirectStatus, readDirectError, readPath, readString, uniqueHTTPURLs } from "./shared";
import type { DirectProtocolAdapter } from "./types";

// Fal.ai 队列协议：POST {base}/{modelId} 提交，得到 request_id 与 response_url；
// 取结果 GET {base}/{modelId}/requests/{requestId}/response：
// 未完成返回 202（空体），完成后返回 200 + 模型输出本体（结构随模型而异）。
// 因为我们提交时用的就是 `{base}/{modelId}`，推导出的结果地址与响应里的 response_url 一致。
// 鉴权头是 `Authorization: Key <FAL_KEY>`，不是 Bearer，因此自定义 authorization。
export const falDirectProtocol: DirectProtocolAdapter = {
    authorization: (apiKey) => (/^key\s/i.test(apiKey.trim()) ? apiKey.trim() : `Key ${apiKey.trim()}`),
    pollPath() {
        throw new Error("Fal 渠道的结果地址需要模型 ID，请通过渠道中的模型发起生成");
    },
    pollURL(baseUrl, taskId, model) {
        const requestId = readString(taskId);
        const modelId = falModelId(model);
        if (!requestId || !modelId) throw new Error("Fal 任务缺少请求 ID 或模型 ID，无法查询结果");
        return `${falBaseURL(baseUrl)}/${modelId}/requests/${encodeURIComponent(requestId)}/response`;
    },
    readTaskId: (payload) => firstString(readPath(payload, "request_id"), readPath(payload, "id")),
    readCreatedVideoStatus: (payload) => normalizeFalStatus(readString(readPath(payload, "status"))),
    readError: readFalError,
    readCreatedImageURLs: readFalImageURLs,
    readImagePoll(payload) {
        const urls = readFalImageURLs(payload);
        const error = urls.length ? "" : readFalErrorIfAny(payload);
        return {
            urls,
            done: urls.length > 0 || normalizeFalStatus(readString(readPath(payload, "status"))) === "completed",
            error,
        };
    },
    readVideoPoll(payload, pollId, model) {
        const videoUrl = readFalVideoURL(payload);
        const error = videoUrl ? "" : readFalErrorIfAny(payload);
        return {
            id: pollId,
            task_id: pollId,
            status: videoUrl ? "completed" : error ? "failed" : normalizeFalStatus(readString(readPath(payload, "status"))),
            ...(videoUrl ? { video_url: videoUrl, url: videoUrl } : {}),
            ...(error ? { error: { message: error } } : {}),
            model,
        };
    },
    readAudioPoll(payload) {
        const url = readFalAudioURL(payload);
        return { url, done: Boolean(url), error: url ? "" : readFalErrorIfAny(payload) };
    },
};

// 模型 ID 支持用 query 追加模型专属参数，例如
// `fal-ai/flux/dev?image_size=landscape_16_9&num_inference_steps=28`；
// URL 与轮询只使用 `?` 之前的真实模型路径。
export function falModelId(model?: string) {
    return readString(model).split(/[?#]/)[0].trim();
}

function falBaseURL(baseUrl: string) {
    return readString(baseUrl).replace(/\/+$/, "");
}

function normalizeFalStatus(value: string) {
    switch (value.trim().toUpperCase()) {
        case "COMPLETED":
        case "OK":
            return "completed";
        case "FAILED":
        case "ERROR":
            return "failed";
        default:
            return normalizeDirectStatus(value);
    }
}

// 只在报文里确实出现错误字段时才解读，避免把模型输出里的普通 message 当成错误。
function hasFalErrorField(payload: unknown) {
    return readPath(payload, "detail") !== undefined || readPath(payload, "error") !== undefined || readPath(payload, "error_type") !== undefined;
}

function readFalErrorIfAny(payload: unknown) {
    return hasFalErrorField(payload) ? readFalError(payload) : "";
}

function readFalError(payload: unknown) {
    const detail = readPath(payload, "detail");
    if (typeof detail === "string" && detail.trim()) return detail.trim();
    if (Array.isArray(detail)) {
        const messages = detail.map((item) => firstString(readPath(item, "msg"), readPath(item, "message"), item)).filter(Boolean);
        if (messages.length) return messages.join("；");
    }
    const error = firstString(readPath(payload, "error.message"), readPath(payload, "error"), readPath(payload, "error_type"), readPath(payload, "message"));
    if (error) return error;
    return readDirectError(payload);
}

function readFalImageURLs(payload: unknown) {
    const record = asRecord(payload);
    return uniqueHTTPURLs([...outputURLs(record.images), ...outputURLs(record.image)]);
}

function readFalVideoURL(payload: unknown) {
    const record = asRecord(payload);
    return firstHTTPURL([outputURLs(record.video), outputURLs(record.videos), outputURLs(record.video_url)]);
}

function readFalAudioURL(payload: unknown) {
    const record = asRecord(payload);
    return firstHTTPURL([outputURLs(record.audio), outputURLs(record.audio_url)]);
}

// 只读取已知的输出字段：提交响应里的 status_url / response_url 也是合法 URL，
// 用通用递归收集会把它们误当成产物地址。
function outputURLs(value: unknown): string[] {
    if (typeof value === "string") return /^https?:\/\//i.test(value.trim()) ? [value.trim()] : [];
    if (Array.isArray(value)) return value.flatMap(outputURLs);
    const record = asRecord(value);
    if (!("url" in record)) return [];
    return outputURLs(record.url);
}
