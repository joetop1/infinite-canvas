"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { imageToDataUrl, type UploadedImage } from "@/services/image-storage";
import { persistDirectorImages, restoreDirectorImages } from "../utils/canvas-director-media";

import type { CanvasDirectorCapture, CanvasDirectorPanorama, CanvasDirectorVideo } from "../types";

type PanoramaRemoval = Pick<CanvasDirectorPanorama, "edgeId" | "sourceNodeId">;
type DirectorCapturePayload = { dataUrl?: unknown; fileName?: unknown } | null | undefined;

export function CanvasDirector({
    nodeId,
    project,
    panoramas,
    theme,
    onClose,
    onProjectChange,
    onPanoramaRemoved,
    onCapturesSent,
    onVideoSent,
}: {
    nodeId: string;
    project: unknown;
    panoramas: CanvasDirectorPanorama[];
    theme: "light" | "dark";
    onClose: () => void;
    onProjectChange: (project: unknown) => void;
    onPanoramaRemoved: (payload: PanoramaRemoval) => void;
    onCapturesSent: (nodeId: string, captures: CanvasDirectorCapture[]) => void;
    onVideoSent: (nodeId: string, video: CanvasDirectorVideo) => void;
}) {
    const iframeRef = useRef<HTMLIFrameElement>(null);
    const projectRef = useRef(project);
    const themeRef = useRef(theme);
    const sessionSentRef = useRef(false);
    const [ready, setReady] = useState(0);
    const [mediaError, setMediaError] = useState("");
    const pendingImages = useRef(new Map<string, Promise<UploadedImage>>());
    const saveRevision = useRef(0);
    const panoramaKey = JSON.stringify(panoramas);

    useEffect(() => {
        projectRef.current = project;
    }, [project]);

    useEffect(() => {
        themeRef.current = theme;
    }, [theme]);

    const postToDesk = useCallback((type: string, payload: unknown) => {
        iframeRef.current?.contentWindow?.postMessage({ type, payload }, window.location.origin);
    }, []);

    useEffect(() => {
        function handleMessage(event: MessageEvent) {
            if (event.origin !== window.location.origin || event.source !== iframeRef.current?.contentWindow) return;

            const type = event.data?.type;
            if (type === "storyai:director-ready") {
                sessionSentRef.current = false;
                setReady((value) => value + 1);
                return;
            }

            if (type === "storyai:director-close") {
                onClose();
                return;
            }

            if (type === "storyai:director-project-changed") {
                const nextProject = event.data?.payload?.project;
                if (nextProject && typeof nextProject === "object" && !Array.isArray(nextProject)) {
                    const revision = ++saveRevision.current;
                    void persistDirectorImages(nextProject, pendingImages.current).then((saved) => {
                        if (revision !== saveRevision.current) return;
                        projectRef.current = saved;
                        onProjectChange(saved);
                        setMediaError("");
                    }).catch((error) => {
                        if (revision === saveRevision.current) setMediaError(`全景图保存失败：${error instanceof Error ? error.message : "请重新导入原图"}`);
                    });
                }
                return;
            }

            if (type === "storyai:director-panorama-removed") {
                const edgeId = typeof event.data?.payload?.edgeId === "string" ? event.data.payload.edgeId.trim() : "";
                const sourceNodeId = typeof event.data?.payload?.sourceNodeId === "string" ? event.data.payload.sourceNodeId.trim() : "";
                if (edgeId && sourceNodeId) onPanoramaRemoved({ edgeId, sourceNodeId });
                return;
            }

            if (type === "storyai:director-captures-sent") {
                const captures = Array.isArray(event.data?.payload?.captures)
                    ? event.data.payload.captures
                        .filter((capture: DirectorCapturePayload): capture is { dataUrl: string; fileName?: unknown } => typeof capture?.dataUrl === "string" && capture.dataUrl.startsWith("data:image/"))
                        .map((capture: { dataUrl: string; fileName?: unknown }, index: number) => ({ dataUrl: capture.dataUrl, fileName: typeof capture.fileName === "string" && capture.fileName.trim() ? capture.fileName.trim() : "导演台截图-" + (index + 1) + ".png" }))
                    : [];
                if (captures.length) void onCapturesSent(nodeId, captures);
                return;
            }

            if (type === "storyai:director-video-sent") {
                const blob = event.data?.payload?.blob;
                if (!(blob instanceof Blob) || !blob.size || !blob.type.startsWith("video/mp4")) return;
                const fileName = typeof event.data?.payload?.fileName === "string" && event.data.payload.fileName.trim()
                    ? event.data.payload.fileName.trim()
                    : "导演台视频.mp4";
                const width = Number(event.data?.payload?.width);
                const height = Number(event.data?.payload?.height);
                const durationSeconds = Number(event.data?.payload?.durationSeconds);
                void onVideoSent(nodeId, {
                    blob,
                    fileName,
                    width: Number.isFinite(width) && width > 0 ? width : 1280,
                    height: Number.isFinite(height) && height > 0 ? height : 720,
                    durationSeconds: Number.isFinite(durationSeconds) ? durationSeconds : 0,
                });
                return;
            }
        }

        window.addEventListener("message", handleMessage);
        return () => window.removeEventListener("message", handleMessage);
    }, [nodeId, onCapturesSent, onClose, onPanoramaRemoved, onProjectChange, onVideoSent]);

    useEffect(() => {
        if (!ready) return;
        let cancelled = false;
        void (async () => {
            // Session must precede panoramas: opening a session clears the iframe's host media map.
            const [restored, resolved] = await Promise.all([
                sessionSentRef.current ? undefined : restoreDirectorImages(projectRef.current),
                Promise.all((JSON.parse(panoramaKey) as CanvasDirectorPanorama[]).map(async (panorama) => ({
                    ...panorama,
                    imageUrl: await imageToDataUrl({ storageKey: panorama.storageKey, url: panorama.imageUrl }),
                }))),
            ]);
            if (cancelled) return;
            if (!sessionSentRef.current) {
                postToDesk("storyai:director-session", { instanceId: nodeId, theme: themeRef.current, project: restored });
                sessionSentRef.current = true;
            }
            postToDesk("storyai:director-panoramas", { panoramas: resolved });
            setMediaError("");
        })().catch((error) => {
            if (cancelled) return;
            // Keep the project editable when an old temporary URL can no longer be recovered.
            if (!sessionSentRef.current) {
                postToDesk("storyai:director-session", { instanceId: nodeId, theme: themeRef.current, project: projectRef.current });
                sessionSentRef.current = true;
            }
            setMediaError(`全景图读取失败：${error instanceof Error ? error.message : "请重新导入原图"}`);
        });
        return () => { cancelled = true; };
    }, [nodeId, panoramaKey, postToDesk, ready]);

    return (
        <div className="fixed inset-0 z-[2000]">
            <iframe ref={iframeRef} title="3D导演台" src="/director/index.html" className="block h-full w-full border-0" />
            {mediaError ? <div role="alert" className="absolute bottom-5 left-1/2 max-w-[80vw] -translate-x-1/2 rounded-lg border border-border bg-popover px-4 py-3 text-sm text-popover-foreground shadow-lg">{mediaError}</div> : null}
        </div>
    );
}
