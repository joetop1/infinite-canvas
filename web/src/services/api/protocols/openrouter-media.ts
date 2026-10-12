import { modelMetadataForConfig, openRouterOmniReferences } from "@/lib/openrouter-catalog";
import { imageToDataUrl } from "@/services/image-storage";
import { resolveMediaUrl } from "@/services/file-storage";
import type { AiConfig } from "@/stores/use-config-store";
import type { ReferenceImage } from "@/types/image";
import type { VideoReferenceInput } from "@/services/api/video";

export function openRouterAspectRatio(size: string) {
    if (/^\d+:\d+$/.test(size)) return size;
    const dimensions = size.match(/^(\d+)x(\d+)$/);
    if (!dimensions) return undefined;
    const width = Number(dimensions[1]), height = Number(dimensions[2]);
    let a = width, b = height;
    while (b) { const remainder = a % b; a = b; b = remainder; }
    return a ? `${width / a}:${height / a}` : undefined;
}

export async function openRouterImageBody(config: AiConfig, prompt: string, references: ReferenceImage[], count: number) {
    const metadata = modelMetadataForConfig(config, config.model, "image");
    const limit = metadata?.supported_parameters?.input_references?.max;
    if (limit !== undefined && references.length > limit) throw new Error(`此 OpenRouter 模型最多支持 ${limit} 张参考图片`);
    const maxCount = metadata?.supported_parameters?.n?.max;
    if (maxCount !== undefined && count > maxCount) throw new Error(`此 OpenRouter 模型每次最多生成 ${maxCount} 张图片`);
    const aspect_ratio = openRouterAspectRatio(config.size);
    const ratios = metadata?.supported_parameters?.aspect_ratio?.values;
    if (aspect_ratio && ratios?.length && !ratios.includes(aspect_ratio)) throw new Error(`此模型支持的比例：${ratios.join("、")}`);
    return { model: config.model, prompt, n: count, ...(aspect_ratio ? { aspect_ratio } : {}), ...(metadata?.supported_parameters?.quality ? { quality: config.quality } : {}), ...(references.length ? { input_references: await Promise.all(references.map(async (reference) => ({ type: "image_url", image_url: { url: await imageToDataUrl(reference) } }))) } : {}) };
}

export async function openRouterVideoBodyFromInput(config: AiConfig, model: string, prompt: string, input: Required<VideoReferenceInput>) {
    const metadata = modelMetadataForConfig(config, model);
    const duration = Number(config.videoSeconds);
    if (!Number.isInteger(duration) || duration <= 0) throw new Error("OpenRouter 视频时长必须是正整数秒");
    if (metadata?.supported_durations?.length && !metadata.supported_durations.includes(duration)) throw new Error(`此模型支持的时长：${metadata.supported_durations.join("、")} 秒`);
    const aspect_ratio = openRouterAspectRatio(config.size);
    if (aspect_ratio && metadata?.supported_aspect_ratios?.length && !metadata.supported_aspect_ratios.includes(aspect_ratio)) throw new Error(`此模型支持的比例：${metadata.supported_aspect_ratios.join("、")}`);
    const requestedResolution = config.vquality.toLowerCase().replace(/p$/, "");
    const resolution = metadata?.supported_resolutions?.find((value) => value.toLowerCase().replace(/p$/, "") === requestedResolution) || (/k$/i.test(requestedResolution) ? requestedResolution.toUpperCase() : `${requestedResolution}p`);
    if (metadata?.supported_resolutions?.length && !metadata.supported_resolutions.includes(resolution)) throw new Error(`此模型支持的分辨率：${metadata.supported_resolutions.join("、")}`);
    if ((input.videoReferences.length || input.audioReferences.length) && !openRouterOmniReferences(model)) throw new Error("此 OpenRouter 模型尚未确认支持视频／音频参考，已阻止提交");
    if ((input.firstFrame || input.lastFrame) && (input.references.length || input.videoReferences.length || input.audioReferences.length)) throw new Error("首尾帧与普通参考素材不能同时提交，请选择一种生成方式");
    for (const [reference, kind] of [[input.firstFrame, "first_frame"], [input.lastFrame, "last_frame"]] as const) {
        if (reference && metadata?.supported_frame_images && !metadata.supported_frame_images.includes(kind)) throw new Error(`此模型不支持${kind === "first_frame" ? "首帧" : "尾帧"}图片`);
    }
    const image = async (reference: ReferenceImage) => ({ type: "image_url", image_url: { url: await imageToDataUrl(reference) } });
    const media = async (reference: { storageKey?: string; url?: string }, kind: "video" | "audio") => {
        const url = await resolveMediaUrl(reference.storageKey, reference.url);
        if (!/^https?:\/\//i.test(url)) throw new Error("OpenRouter 视频／音频参考需要可公开访问的素材地址，请配置对象存储后上传");
        return { type: `${kind}_url`, [`${kind}_url`]: { url } };
    };
    const references = await Promise.all([...input.references.map(image), ...input.videoReferences.map((item) => media(item, "video")), ...input.audioReferences.map((item) => media(item, "audio"))]);
    const frames = await Promise.all(([ [input.firstFrame, "first_frame"], [input.lastFrame, "last_frame"] ] as const).filter(([reference]) => reference).map(async ([reference, frame_type]) => ({ ...await image(reference!), frame_type })));
    return { model, prompt, duration, resolution, ...(aspect_ratio ? { aspect_ratio } : {}), ...(typeof metadata?.generate_audio === "boolean" ? { generate_audio: metadata.generate_audio && config.videoGenerateAudio === "true" } : {}), ...(references.length ? { input_references: references } : {}), ...(frames.length ? { frame_images: frames } : {}) };
}
