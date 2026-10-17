import { mediaContractError } from "@/lib/media-contract";
import type { ProviderModelMetadata } from "@/lib/openrouter-catalog";

export function imageModelDisplayName(id: string, metadata?: ProviderModelMetadata) {
    if (metadata?.name) return metadata.name.replace(/^[^:]+:\s*/, "");
    const name = id.split(/[?#]/)[0].replace(/^[^/]+\//, "").replace(/\/(?:edit|image-to-image)$/, "");
    return name.replace(/[\/_]+/g, " · ").replace(/-/g, " ").replace(/\b[a-z]/g, (letter) => letter.toUpperCase()).replace(/\bFlux\b/g, "FLUX");
}

function referenceSupport(metadata?: ProviderModelMetadata) {
    const contract = metadata?.generation_contract;
    if (contract) return Boolean(contract.bindings?.images);
    const input = metadata?.architecture?.input_modalities;
    return input?.length ? input.includes("image") : undefined;
}

export function imageRatioSupport(metadata: ProviderModelMetadata | undefined, ratio: string) {
    const field = metadata?.generation_contract?.bindings?.ratio;
    const values = field?.values?.map(String) || metadata?.supported_aspect_ratios;
    if (values?.length) return values.includes(ratio);
    // A free-form string or a pixel-size object does not prove a particular ratio is supported.
    return undefined;
}

export function imageModelPickerTags(metadata?: ProviderModelMetadata, panorama = false) {
    const contract = metadata?.generation_contract;
    if (contract && contract.status !== "ready") return [contract.status === "syncing" ? "规范同步中" : "待适配", contract.reason || "暂不可生成"];
    const fields = contract?.bindings || {};
    const resolutions = (fields.resolution?.values || metadata?.supported_resolutions || []).map(String);
    const numeric = resolutions.filter((value) => /^\d+(?:\.\d+)?k$/i.test(value)).sort((a, b) => parseFloat(a) - parseFloat(b));
    const resolution = numeric.length ? `${numeric.length > 1 ? "最高 " : ""}${numeric.at(-1)!.toUpperCase()}` : resolutions.length ? resolutions.join(" / ") : fields.size?.type === "object" || fields.width && fields.height ? "可设像素尺寸" : "清晰度待确认";
    const support = referenceSupport(metadata);
    const max = fields.images?.maxItems;
    const reference = support === false ? "仅文生图" : support === true ? max ? `最多 ${max} 张参考` : "支持参考图" : "参考能力待确认";
    const ratios = fields.ratio?.values || metadata?.supported_aspect_ratios;
    const panoramaSupport = imageRatioSupport(metadata, "2:1");
    const ratio = panoramaSupport === true ? "支持 2:1" : panorama ? panoramaSupport === false ? "不支持 2:1" : "2:1 待确认" : ratios?.length ? `${ratios.filter((value) => !["auto", "adaptive"].includes(String(value))).length} 种比例` : "比例待确认";
    return [resolution, reference, ratio];
}

export function imageModelCapabilityError(metadata: ProviderModelMetadata | undefined, input: { hasReferences?: boolean; referenceCount?: number; panorama?: boolean; ratio?: string } = {}) {
    const unavailable = mediaContractError(metadata);
    if (unavailable) return unavailable;
    const field = metadata?.generation_contract?.bindings?.images;
    if (input.hasReferences && referenceSupport(metadata) === false) return "此模型不支持参考图片，请选择支持图生图的模型";
    if (!input.hasReferences && field?.required) return "此模型需要参考图片，请先添加图片";
    if (field?.maxItems && (input.referenceCount || 0) > field.maxItems) return `此模型最多支持 ${field.maxItems} 张参考图片`;
    const ratio = input.panorama ? "2:1" : input.ratio;
    if (ratio && imageRatioSupport(metadata, ratio) === false) return input.panorama ? "此模型不支持全景图所需的 2:1 比例，请更换模型" : `此模型不支持 ${ratio} 比例，请重新选择尺寸`;
    return "";
}
