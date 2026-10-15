"use client";

import { App, Button, Modal } from "antd";
import { useEffect, useRef, useState } from "react";
import { canvasThemes } from "@/lib/canvas-theme";
import { readMediaMetadata } from "@/lib/media-metadata";
import { useThemeStore } from "@/stores/use-theme-store";
import { downloadRemoteMedia, resolveMediaUrl, uploadMediaFile, type UploadedFile } from "@/services/file-storage";
import { trimVideoFile } from "@/services/api/video-trim";
import type { CanvasNodeData } from "../types";

export function CanvasVideoTrimDialog({ source, onClose, onComplete }: { source: CanvasNodeData; onClose: () => void; onComplete: (file: UploadedFile, start: number, end: number) => void }) {
    const { message } = App.useApp();
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const [url, setUrl] = useState("");
    const [duration, setDuration] = useState(0);
    const [start, setStart] = useState(0);
    const [end, setEnd] = useState(0);
    const [busy, setBusy] = useState(false);
    const controller = useRef<AbortController | null>(null);
    const player = useRef<HTMLVideoElement | null>(null);
    useEffect(() => {
        let active = true;
        resolveMediaUrl(source.metadata?.storageKey, source.metadata?.content).then(async (src) => {
            const meta = await readMediaMetadata(src, "video");
            if (!active) return;
            setUrl(src); setDuration(meta.durationMs / 1000); setEnd(Math.min(14.9, meta.durationMs / 1000));
        }).catch((error) => { if (active) message.error(error instanceof Error ? error.message : "无法读取视频"); });
        return () => { active = false; controller.current?.abort(); };
    }, [source.id, source.metadata?.storageKey, source.metadata?.content, message]);
    const selectedMs = Math.round((end-start)*1000);
    const valid = Number.isFinite(start) && Number.isFinite(end) && start >= 0 && end <= duration && selectedMs >= 3000 && selectedMs <= 14900;
    const save = async () => {
        if (!valid || busy) return;
        setBusy(true);
        const abort = new AbortController(); controller.current = abort;
        try {
            const input = await downloadRemoteMedia(url);
            if (abort.signal.aborted) return;
            const result = await trimVideoFile(input, start, end, abort.signal);
            const file = await uploadMediaFile(result, "trimmed-video");
            if (abort.signal.aborted) return;
            onComplete(file, start, end);
        } catch (error) { if (!abort.signal.aborted) message.error(error instanceof Error ? error.message : "裁剪失败"); }
        finally { if (!abort.signal.aborted) setBusy(false); }
    };
    return <Modal open title="裁剪原视频用于编辑" width={760} footer={null} onCancel={() => { if (!busy) onClose(); }} maskClosable={!busy} closable={!busy}>
        {url ? <video ref={player} src={url} controls preload="metadata" className="my-3 max-h-[340px] w-full rounded-xl" onTimeUpdate={() => { if (player.current && !player.current.paused && player.current.currentTime >= end) { player.current.pause(); player.current.currentTime = start; } }} /> : <div className="py-8">正在读取原视频…</div>}
        <div className="mb-4 text-sm" style={{ color: theme.node.muted }}>原片实际长度：{duration.toFixed(3)} 秒。选取 3–14.9 秒片段，为音频和尾帧保留余量；另存为新节点，原视频保留。</div>
        <div className="flex flex-wrap items-center gap-4">
            <label className="text-sm">开始（秒） <input aria-label="裁剪开始时间" type="number" step="0.1" min="0" max={duration} value={start} disabled={busy} className="w-24 rounded-lg border p-2" style={{ background: theme.node.fill, borderColor: theme.node.stroke, color: theme.node.text }} onChange={(e) => setStart(Number(e.target.value))} /></label>
            <label className="text-sm">结束（秒） <input aria-label="裁剪结束时间" type="number" step="0.1" min="0" max={duration} value={end} disabled={busy} className="w-24 rounded-lg border p-2" style={{ background: theme.node.fill, borderColor: theme.node.stroke, color: theme.node.text }} onChange={(e) => setEnd(Number(e.target.value))} /></label>
            <span className="text-sm">选中 {Math.max(0, end-start).toFixed(3)} 秒</span>
        </div>
        <div className="mt-5 flex justify-end gap-2">
            <Button onClick={() => { controller.current?.abort(); onClose(); }}>取消</Button>
            <Button disabled={!valid || busy || !url} onClick={() => { if (player.current) { player.current.currentTime = start; void player.current.play(); } }}>预览片段</Button>
            <Button type="primary" loading={busy} disabled={!valid || !url} onClick={() => void save()}>裁剪另存</Button>
        </div>
    </Modal>;
}
