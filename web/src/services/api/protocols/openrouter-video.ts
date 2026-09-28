import { readFileAsDataUrl } from "@/lib/image-utils";

export function isOpenRouterBaseURL(baseUrl: string) {
    try { return new URL(baseUrl).hostname.toLowerCase() === "openrouter.ai"; } catch { return false; }
}

// 未登录的本地渠道直接调用上游，也需要把表单转换成 OpenRouter 视频 JSON。
export async function openRouterVideoBody(body: FormData) {
    const out: Record<string, unknown> = {};
    for (const key of ["model", "prompt", "size"]) {
        const value = body.get(key);
        if (typeof value === "string" && value) out[key] = value;
    }
    const seconds = body.get("seconds");
    if (seconds !== null) {
        const duration = Number(seconds);
        if (!Number.isInteger(duration) || duration < 1) throw new Error("OpenRouter 视频时长必须是正整数秒");
        out.duration = duration;
    }
    if (body.has("resolution_name")) out.resolution = body.get("resolution_name");
    if (body.has("video_generate_audio")) out.generate_audio = body.get("video_generate_audio") === "true";
    const media = async (value: FormDataEntryValue, kind: string) => ({ type: `${kind}_url`, [`${kind}_url`]: { url: typeof value === "string" ? value : await readFileAsDataUrl(value) } });
    const references = await Promise.all(["input_reference[]", "video_reference[]", "audio_reference[]"].flatMap((key, index) => body.getAll(key).map(value => media(value, ["image", "video", "audio"][index]))));
    if (references.length) out.input_references = references;
    const frames = await Promise.all(["first_frame", "last_frame"].flatMap(kind => body.getAll(`${kind}_url`).map(async value => ({ ...await media(value, "image"), frame_type: kind }))));
    if (frames.length) out.frame_images = frames;
    return out;
}
