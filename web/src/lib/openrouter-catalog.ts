export type ProviderModelMetadata = {
    name?: string;
    architecture?: { input_modalities?: string[]; output_modalities?: string[] };
    supported_resolutions?: string[];
    supported_durations?: number[];
    supported_aspect_ratios?: string[];
    supported_frame_images?: string[];
    generate_audio?: boolean | null;
    supported_parameters?: Record<string, { type?: string; values?: string[]; min?: number; max?: number }>;
};
export type ModelMetadata = Record<string, ProviderModelMetadata>;
export type ChannelModelCatalog = { models: string[]; modelCapabilities: Record<string, "text" | "image" | "video" | "audio">; modelMetadata: ModelMetadata };

export async function fetchOpenRouterCatalog(baseUrl: string, apiKey: string): Promise<ChannelModelCatalog> {
    const base = baseUrl.replace(/\/+$/, "");
    const catalogs = await Promise.all(["models", "images/models", "videos/models"].map(async (path) => {
        const response = await fetch(`${base}/${path}`, { headers: { Authorization: `Bearer ${apiKey}` }, signal: AbortSignal.timeout(30000) });
        if (!response.ok) throw new Error(`OpenRouter 模型目录读取失败：${response.status}`);
        const result = await response.json();
        if (!Array.isArray(result.data)) throw new Error("OpenRouter 模型目录返回格式异常");
        return result.data as Array<ProviderModelMetadata & { id: string }>;
    }));
    const result: ChannelModelCatalog = { models: [], modelCapabilities: {}, modelMetadata: {} };
    catalogs.forEach((rows, index) => rows.forEach((item) => {
        if (!item.id) return;
        const output = item.architecture?.output_modalities || [];
        const kind = index === 2 ? "video" : index === 1 ? "image" : output.includes("video") ? "video" : output.includes("image") ? "image" : output.includes("text") ? "text" : undefined;
        if (!kind) return;
        result.modelCapabilities[item.id] = kind;
        result.modelMetadata[item.id] = item;
    }));
    result.models = Object.keys(result.modelCapabilities).sort();
    return result;
}

export function modelMetadataForConfig(config: { channelMode: string; publicChannels: Array<{ id?: string; models?: string[]; modelMetadata?: ModelMetadata }>; localChannels: Array<{ id?: string; models?: string[]; modelMetadata?: ModelMetadata }>; activeChannelId?: string; videoChannelId?: string; imageChannelId?: string }, model: string, kind: "video" | "image" = "video") {
    const channels = config.channelMode === "remote" ? config.publicChannels : config.localChannels;
    const id = (kind === "video" ? config.videoChannelId : config.imageChannelId) || config.activeChannelId;
    const channel = channels.find((item) => item.id === id && item.models?.includes(model)) || channels.find((item) => item.models?.includes(model));
    return channel?.modelMetadata?.[model];
}

export function openRouterOmniReferences(model: string) {
    return /^bytedance(?:-seed)?\/seedance-2(?:\.0)?(?:$|-)/i.test(model);
}
