import { imageToDataUrl, uploadImage, type UploadedImage } from "@/services/image-storage";

type DirectorAsset = Record<string, unknown>;
type DirectorProject = Record<string, unknown> & { assets: DirectorAsset[] };

function imageAssets(project: unknown): project is DirectorProject {
    return Boolean(project && typeof project === "object" && Array.isArray((project as DirectorProject).assets));
}

// Persist bytes while the importing iframe still owns its temporary URL.
export async function persistDirectorImages(project: unknown, pending: Map<string, Promise<UploadedImage>>) {
    if (!imageAssets(project)) return project;
    const assets = await Promise.all(project.assets.map(async (asset) => {
        if (asset.sourceType !== "image") return asset;
        if (typeof asset.storageKey === "string" && asset.storageKey) return { ...asset, url: "" };
        const url = typeof asset.url === "string" ? asset.url : "";
        if (!url.startsWith("blob:") && !url.startsWith("data:image/")) return asset;
        let saved = pending.get(url);
        if (!saved) {
            saved = fetch(url).then(async (response) => {
                if (!response.ok) throw new Error("无法读取导入的全景图片，请重新导入原图");
                return uploadImage(await response.blob());
            });
            pending.set(url, saved);
            saved.catch(() => pending.delete(url));
        }
        try {
            const image = await saved;
            return { ...asset, storageKey: image.storageKey, url: "" };
        } catch (error) {
            // An unused legacy broken image must not prevent saving its replacement.
            if (asset.id !== project.panoramaAssetId) return { ...asset, url: "" };
            throw error;
        }
    }));
    return { ...project, assets };
}

// Data URLs are session-only; the saved project keeps the durable storage key.
export async function restoreDirectorImages(project: unknown) {
    if (!imageAssets(project)) return project;
    const assets = await Promise.all(project.assets.map(async (asset) => {
        if (asset.sourceType !== "image") return asset;
        if (typeof asset.storageKey !== "string" || !asset.storageKey) {
            if (typeof asset.url === "string" && asset.url.startsWith("blob:")) {
                const response = await fetch(asset.url).catch(() => null);
                if (!response?.ok) {
                    if (asset.id !== project.panoramaAssetId) return { ...asset, url: "" };
                    throw new Error(`旧临时图片已失效，请重新导入：${asset.fileName || "全景图"}`);
                }
            }
            return asset;
        }
        try {
            const url = await imageToDataUrl({ storageKey: asset.storageKey });
            if (!url) throw new Error(`无法恢复全景图片：${asset.fileName || "未知图片"}`);
            return { ...asset, url };
        } catch (error) {
            if (asset.id !== project.panoramaAssetId) return { ...asset, url: "" };
            throw error;
        }
    }));
    return { ...project, assets };
}
