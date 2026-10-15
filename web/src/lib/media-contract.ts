import type { MediaContract, ProviderModelMetadata } from "@/lib/openrouter-catalog";

export function mediaContractError(metadata?: ProviderModelMetadata) {
    const contract = metadata?.generation_contract;
    if (!contract || contract.status === "ready") return "";
    return contract.reason || "此模型尚未完成适配，请在渠道中同步模型规范";
}

export function mediaContractReferences(contract: MediaContract, references: { hasImageReferences?: boolean; hasVideoReferences?: boolean; hasAudioReferences?: boolean }) {
    const fields = contract.bindings || {};
    if (references.hasImageReferences && !fields.images && !fields.first_frame) return "此模型不支持已适配的参考图片输入";
    if (references.hasVideoReferences && !fields.videos) return "此模型不支持已适配的参考视频输入";
    if (references.hasAudioReferences && !fields.audios) return "此模型不支持已适配的参考音频输入";
    return "";
}

export function mediaContractReferenceError(metadata: ProviderModelMetadata | undefined, input: { references: unknown[]; videoReferences: Array<{ durationMs?: number }>; audioReferences: Array<{ durationMs?: number }>; firstFrame?: unknown; lastFrame?: unknown }) {
    const contract = metadata?.generation_contract;
    if (!contract) return "";
    const error = mediaContractError(metadata) || mediaContractReferences(contract, { hasImageReferences: input.references.length > 0, hasVideoReferences: input.videoReferences.length > 0, hasAudioReferences: input.audioReferences.length > 0 });
    if (error) return error;
    const fields = contract.bindings || {};
    const count = input.references.length + input.videoReferences.length + input.audioReferences.length;
    if (contract.requiresReference && count === 0 && !input.firstFrame) return "此模型需要至少一份参考素材";
    if (contract.maxReferences && count > contract.maxReferences) return `参考素材总数不能超过 ${contract.maxReferences}`;
    for (const [role, label, items] of [["images", "参考图片", input.references], ["videos", "参考视频", input.videoReferences], ["audios", "参考音频", input.audioReferences]] as const) {
        const max = fields[role]?.maxItems;
        if (max && items.length > max) return `${label}最多支持 ${max} 份`;
    }
    for (const [label, items] of [["参考视频", input.videoReferences], ["参考音频", input.audioReferences]] as const) {
        if (contract.referenceMaxSeconds && items.some((item) => !item.durationMs || item.durationMs < (contract.referenceMinSeconds || 0) * 1000 || item.durationMs > contract.referenceMaxSeconds! * 1000)) return `${label}每段需要在 ${contract.referenceMinSeconds || 0}–${contract.referenceMaxSeconds} 秒之间`;
        if (contract.referenceTotalSeconds && items.reduce((sum, item) => sum + (item.durationMs || 0), 0) > contract.referenceTotalSeconds * 1000) return `${label}总时长不能超过 ${contract.referenceTotalSeconds} 秒`;
    }
    if (input.lastFrame && !fields.last_frame) return "此模型没有已适配的尾帧参数";
    return "";
}
