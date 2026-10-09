import { ARK_SEEDANCE_REFERENCE_LIMITS, SEEDANCE_REFERENCE_LIMITS, seedanceVideoReferenceError } from "@/lib/seedance-video";
import { channelProtocolForConfig, type AiConfig } from "@/stores/use-config-store";
import type { VideoReferenceInput } from "@/services/api/video";

// Only enable modes for adapters that transmit all three reference media types.
export function omniReferenceLimits(config: AiConfig) {
    if (config.videoWorkflowRef) return null;
    const model = config.model || config.videoModel;
    const protocol = channelProtocolForConfig({ ...config, model, videoModel: model });
    if (!/^doubao-seedance-2-(?:0|5)(?:-|$)/i.test(model) || !["ark", "openai"].includes(protocol)) return null;
    return protocol === "ark" && /seedance-2-5/i.test(model) ? ARK_SEEDANCE_REFERENCE_LIMITS : SEEDANCE_REFERENCE_LIMITS;
}

export function selectVideoReferenceMode(config: AiConfig, input: Required<VideoReferenceInput>): Required<VideoReferenceInput> {
    const limits = omniReferenceLimits(config);
    if (!limits) return input;
    const mode = config.videoReferenceMode || (input.firstFrame || input.lastFrame ? "frames" : "omni");
    if (mode === "frames") {
        if (!input.firstFrame) throw new Error("首尾帧模式需要指定首帧图片");
        return { ...input, references: [], videoReferences: [], audioReferences: [] };
    }
    const selected = { ...input, firstFrame: null, lastFrame: null };
    if (config.videoReferenceMode === "omni" && !input.references.length && !input.videoReferences.length) throw new Error("全能参考需要添加参考图片或参考视频");
    if (input.audioReferences.length && !input.references.length && !input.videoReferences.length) throw new Error("参考音频需要同时提供参考图片或参考视频");
    if (input.references.length > limits.images || input.videoReferences.length > limits.videos || input.audioReferences.length > limits.audios) throw new Error(`全能参考最多支持 ${limits.images} 张图片、${limits.videos} 个视频、${limits.audios} 个音频`);
    let audioDurationMs = 0;
    for (const audio of input.audioReferences) {
        if (audio.durationMs && (audio.durationMs < 2000 || audio.durationMs > limits.maxDurationMs)) throw new Error(`参考音频时长需要在 2–${limits.maxDurationMs / 1000} 秒之间`);
        audioDurationMs += audio.durationMs || 0;
    }
    if (audioDurationMs > limits.totalDurationMs) throw new Error(`参考音频总时长不能超过 ${limits.totalDurationMs / 1000} 秒`);
    const videoError = seedanceVideoReferenceError(input.videoReferences, limits);
    if (videoError) throw new Error(videoError);
    return selected;
}
