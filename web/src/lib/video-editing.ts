import { normalizeLocalChannels, type AiConfig } from "@/stores/use-config-store";
import type { VideoReferenceInput } from "@/services/api/video";

export const VIDEO_EDIT_MODELS = [
    "fal-ai/kling-video/o3/pro/video-to-video/edit",
    "fal-ai/kling-video/o3/standard/video-to-video/edit",
];

export function supportsVideoEditing(model: string, protocol: string) {
    return protocol === "fal" && VIDEO_EDIT_MODELS.includes(model);
}

export function videoEditingOptions(config: AiConfig) {
    const channels = config.channelMode === "remote" ? config.publicChannels : normalizeLocalChannels(config);
    return channels.flatMap((channel) => (channel.models || []).filter((model) => supportsVideoEditing(model, channel.protocol || "")).map((model) => ({ model, channelId: channel.id || "" })));
}

export function preferredVideoEditor(config: AiConfig) {
    const options = videoEditingOptions(config);
    return options.find((item) => item.model === config.videoModel && item.channelId === config.videoChannelId) || options[0];
}

export function videoEditingInputError(input: Required<VideoReferenceInput>) {
    if (input.videoReferences.length !== 1) return "视频编辑需要且只支持一段原视频";
    if (input.references.length > 4) return "Kling O3 编辑最多支持 4 张参考图";
    if (input.audioReferences.length || input.firstFrame || input.lastFrame) return "视频编辑使用原视频和参考图片，请移除音频及首尾帧设置";
    const video = input.videoReferences[0];
    if (!video.durationMs || video.durationMs < 3000 || video.durationMs > 15000) return "Kling O3 编辑的原视频实际长度需要在 3–15 秒之间，请先裁剪超长视频";
    if (video.width && video.height && (Math.min(video.width, video.height) < 720 || Math.max(video.width, video.height) > 3840)) return "Kling O3 编辑的原视频尺寸需在 720–3840 像素范围内";
    return "";
}

export function videoEditingPrompt(prompt: string) {
    return prompt.replace(/@?视频\s*(\d+)/g, "@Video$1").replace(/@?图片\s*(\d+)/g, "@Image$1");
}
