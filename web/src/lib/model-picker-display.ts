import { isFalTextToVideoModel, modelKey, supportsVideoFrameReferences, videoModelSummary } from "@/lib/video-model-capabilities";

export type PickerReferences = { hasImageReferences?: boolean; hasVideoReferences?: boolean; hasAudioReferences?: boolean };

// Display names never replace the provider ID used to submit a request.
export function videoModelDisplayName(id: string) {
    const key = modelKey(id.split(/[?#]/)[0]);
    const families: [RegExp, string][] = [
        [/seedance-(\d+)-(\d+)/, "Seedance"], [/seedance-(\d+)(?!\d)/, "Seedance"],
        [/kling(?:-video)?-v?(\d+)-(\d+)/, "Kling"], [/veo-?(\d+)(?:-(\d+))?/, "Veo"],
        [/hailuo-(\d+)/, "Hailuo"], [/minimax-h(\d+)/, "MiniMax H"],
        [/wan-?(\d+)-(\d+)/, "Wan"], [/flux-(\d+)-(\d+)/, "FLUX"],
    ];
    for (const [pattern, family] of families) {
        const match = key.match(pattern);
        if (!match) continue;
        const version = match[2] ? `${match[1]}.${match[2]}` : match[1];
        const variants = ["master", "standard", "pro", "fast", "mini", "max", "turbo", "light"].filter((word) => new RegExp(`(?:^|-)${word}(?:-|$)`).test(key));
        return `${family}${family.endsWith("H") ? "" : " "}${version}${variants.length ? ` ${variants.map((word) => word[0].toUpperCase() + word.slice(1)).join(" ")}` : ""}`;
    }
    return id.replace(/^(?:fal-ai|bytedance|black-forest-labs|google|minimax|wan-video)\//i, "").replace(/\/(?:text-to-video|image-to-video|reference-to-video|video-to-video)$/i, "").replace(/[\/_]+/g, " · ");
}

function videoReferenceSupport(id: string, protocol: string) {
    const key = modelKey(id);
    const omni = /^(?:doubao-)?seedance-2-(?:0|5)(?:-|$)/.test(key) && ["ark", "openai"].includes(protocol);
    if (omni) return { image: true, video: true, audio: true, label: "全能参考" };
    if (isFalTextToVideoModel(id, protocol) || key.includes("text-to-video") || /(?:^|-)t2v(?:-|$)/.test(key)) return { image: false, video: false, audio: false, label: "文生视频" };
    if (key.includes("motion-control")) return { image: true, video: true, audio: false, label: "动作参考" };
    if (key.includes("video-to-video")) return { video: true, label: "视频参考" };
    if (key.includes("image-to-video") || /(?:^|-)i2v(?:-|$)/.test(key)) return { image: true, video: false, audio: false, label: "图生视频" };
    if (key.includes("reference-to-video")) return { image: true, label: "图片参考" };
    if (supportsVideoFrameReferences(id, protocol)) return { image: true, label: "图片参考" };
    return { label: "参考能力待确认" };
}

export function videoModelPickerTags(id: string, protocol: string) {
    const summary = videoModelSummary(id, protocol);
    const referenceLabel = videoReferenceSupport(id, protocol).label;
    return summary ? [summary.resolution, summary.duration, summary.supportsAudio ? "有声" : "无声", referenceLabel] : ["分辨率／时长／音频待确认", referenceLabel];
}

export function videoModelReferenceError(id: string, protocol: string, references: PickerReferences) {
    const support = videoReferenceSupport(id, protocol);
    if (references.hasImageReferences && support.image === false) return "此模型不支持参考图片，请选择图生视频或全能参考模型";
    if (references.hasVideoReferences && support.video === false) return "此模型不支持参考视频，请选择支持视频参考的模型";
    if (references.hasAudioReferences && support.audio === false) return "此模型不支持参考音频，请选择全能参考模型";
    return "";
}
