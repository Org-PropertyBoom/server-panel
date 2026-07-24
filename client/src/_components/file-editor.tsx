import { useEffect, useState } from "react";
import { FileText, X, Loader2, AlertCircle, FolderOpen, Pencil, Save, Info, ChevronRight } from "lucide-react";
import { toast } from "sonner";

interface FileEditorProps {
    fileName: string;
    filePath: string;
    fileSize: number;
    content: string;
    isBinary: boolean;
    isLoading: boolean;
    error: string | null;
    onClose?: () => void;
    canEdit?: boolean;
    onSave?: (content: string) => Promise<void>;
    onToggleDetails?: () => void;
    detailsOpen?: boolean;
    placeholderTitle?: string;
    placeholderDescription?: string;
}

function formatBytes(bytes: number) {
    if (bytes === 0) return "0 Bytes";
    const k = 1024;
    const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

export default function FileEditor({
    fileName,
    filePath,
    fileSize,
    content,
    isBinary,
    isLoading,
    error,
    onClose,
    canEdit = false,
    onSave,
    onToggleDetails,
    detailsOpen = false,
    placeholderTitle = "Ppt Server Panel Editor",
    placeholderDescription = "Select a configuration file or script from the directory tree sidebar to view or edit its contents.",
}: FileEditorProps) {
    const [editing, setEditing] = useState(false);
    const [draft, setDraft] = useState(content);
    const [confirming, setConfirming] = useState(false);
    const [saving, setSaving] = useState(false);

    // Reset edit state whenever a different file loads or the saved content changes.
    useEffect(() => {
        setEditing(false);
        setDraft(content);
        setConfirming(false);
    }, [filePath, content]);

    const dirty = draft !== content;
    const editable = canEdit && !isBinary && !error && !isLoading && Boolean(onSave);

    const doSave = async () => {
        if (!onSave) return;
        setSaving(true);
        try {
            await onSave(draft);
            toast.success(`${fileName} saved`);
            setConfirming(false);
            setEditing(false);
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Save failed");
        } finally {
            setSaving(false);
        }
    };

    if (!fileName && !filePath) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center p-8 select-none bg-slate-950 text-slate-500">
                <div className="flex flex-col items-center max-w-md text-center gap-6">
                    <div className="h-16 w-16 items-center justify-center rounded-xl bg-slate-900 border border-slate-800 flex text-slate-400">
                        <FolderOpen className="h-8 w-8" />
                    </div>
                    <div className="space-y-1.5">
                        <h3 className="text-slate-300 font-semibold text-sm">{placeholderTitle}</h3>
                        <p className="text-xs text-slate-600 max-w-xs leading-relaxed">{placeholderDescription}</p>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="flex-grow flex flex-col h-full overflow-hidden text-slate-200 bg-slate-950">
            {/* Editor Tab Bar */}
            <div className="flex h-10 items-center border-b border-slate-800 bg-slate-900/60 select-none shrink-0">
                <div className="flex h-full items-center gap-2 px-3 bg-slate-950 border-r border-slate-800 text-xs font-medium text-slate-200 relative">
                    <FileText className="h-3.5 w-3.5 text-slate-400 shrink-0" />
                    <span>{fileName}</span>
                    {dirty ? <span className="h-1.5 w-1.5 rounded-full bg-amber-400" title="Unsaved changes" /> : null}
                    {onClose && (
                        <button onClick={onClose} className="ml-2 p-0.5 rounded text-slate-500 hover:bg-slate-800 hover:text-slate-200 transition-colors">
                            <X className="h-3 w-3" />
                        </button>
                    )}
                </div>
                <div className="flex-1" />
                {editable && !editing ? (
                    <button onClick={() => setEditing(true)} className="mr-2 inline-flex items-center gap-1.5 rounded border border-slate-700 bg-slate-800/60 px-2.5 py-1 text-[11px] text-slate-200 hover:bg-slate-800">
                        <Pencil className="h-3 w-3" /> Edit
                    </button>
                ) : editing ? (
                    <div className="mr-2 flex items-center gap-1.5">
                        <button onClick={() => { setDraft(content); setEditing(false); }} disabled={saving} className="rounded border border-slate-700 px-2.5 py-1 text-[11px] text-slate-300 hover:bg-slate-800">
                            Cancel
                        </button>
                        <button onClick={() => setConfirming(true)} disabled={!dirty || saving} className="inline-flex items-center gap-1.5 rounded bg-emerald-600 px-2.5 py-1 text-[11px] font-medium text-white hover:bg-emerald-500 disabled:opacity-50">
                            <Save className="h-3 w-3" /> Save
                        </button>
                    </div>
                ) : null}
                <div className="px-4 text-[10px] text-slate-500 font-mono">{formatBytes(fileSize)}</div>
                {onToggleDetails ? (
                    <button onClick={onToggleDetails} title={detailsOpen ? "Hide details" : "Show details"} className={`mr-2 rounded p-1 transition-colors ${detailsOpen ? "bg-slate-800 text-slate-200" : "text-slate-500 hover:bg-slate-800 hover:text-slate-200"}`}>
                        <Info className="h-3.5 w-3.5" />
                    </button>
                ) : null}
            </div>

            {/* Breadcrumb (VS Code-style path) */}
            {filePath ? (
                <div className="flex items-center gap-0.5 overflow-x-auto whitespace-nowrap border-b border-slate-800 bg-slate-900/40 px-3 py-1 text-[11px] text-slate-400 select-none">
                    {filePath.split("/").filter(Boolean).map((seg, i, arr) => (
                        <span key={i} className="flex items-center gap-0.5">
                            {i > 0 ? <ChevronRight className="h-3 w-3 shrink-0 text-slate-600" /> : null}
                            <span className={i === arr.length - 1 ? "text-slate-200" : ""}>{seg}</span>
                        </span>
                    ))}
                </div>
            ) : null}

            {/* Editor Content Area */}
            <div className="flex-1 overflow-hidden relative">
                {isLoading ? (
                    <div className="flex h-full items-center justify-center bg-slate-950">
                        <Loader2 className="h-8 w-8 animate-spin text-slate-500" />
                    </div>
                ) : error ? (
                    <div className="flex flex-col items-center justify-center h-full gap-2 text-red-400 p-6 text-center select-none bg-slate-950">
                        <AlertCircle className="h-8 w-8 shrink-0" />
                        <p className="text-sm font-semibold">{error}</p>
                    </div>
                ) : isBinary ? (
                    <div className="flex flex-col items-center justify-center h-full text-slate-500 p-8 select-none bg-slate-950">
                        <AlertCircle className="h-10 w-10 mb-3 text-amber-600 animate-pulse" />
                        <p className="text-sm font-semibold text-slate-300">Binary file not displayed</p>
                        <p className="text-xs mt-1 max-w-sm text-center leading-relaxed">
                            This file is not displayed in the text editor because it is either binary, has an unsupported text encoding, or is too large.
                        </p>
                    </div>
                ) : editing ? (
                    <textarea
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                        spellCheck={false}
                        autoComplete="off"
                        className="h-full w-full resize-none bg-slate-950 p-4 font-mono text-xs md:text-sm leading-6 text-slate-200 outline-none"
                        aria-label={`Edit ${fileName}`}
                    />
                ) : (
                    <div className="flex font-mono text-xs md:text-sm leading-6 overflow-auto h-full bg-slate-950 text-slate-300">
                        <div className="text-right pr-4 pl-3 select-none text-slate-600 border-r border-slate-800 bg-slate-900/20 sticky left-0 min-w-[3.5rem] py-4 shrink-0">
                            {content.split("\n").map((_, idx) => (
                                <div key={idx} className="h-6">{idx + 1}</div>
                            ))}
                        </div>
                        <pre className="pl-4 pr-6 py-4 m-0 select-text whitespace-pre overflow-visible flex-1 leading-6">{content}</pre>
                    </div>
                )}
            </div>

            {/* Editor Status Bar */}
            <div className="border-t border-slate-900 bg-slate-900 px-4 py-1.5 flex items-center justify-between text-[10px] text-slate-500 font-mono select-none shrink-0">
                <span className="truncate max-w-md" title={filePath}>Path: {filePath}</span>
                <span>{editing ? "EDITING · UTF-8" : "UTF-8"}</span>
            </div>

            {/* Save confirmation */}
            {confirming ? (
                <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => (saving ? null : setConfirming(false))}>
                    <div className="w-full max-w-md rounded-lg border border-slate-700 bg-slate-900 p-5 text-slate-200 shadow-xl" onClick={(e) => e.stopPropagation()}>
                        <h2 className="text-sm font-semibold">Save changes to this file?</h2>
                        <p className="mt-2 text-xs text-slate-400">
                            This overwrites the file on the server. The previous contents are backed up to
                            <code className="mx-1 rounded bg-slate-800 px-1">{fileName}.bak-&lt;timestamp&gt;</code>
                            in the same folder, so you can restore if needed.
                        </p>
                        <p className="mt-2 break-all font-mono text-[11px] text-slate-400">{filePath}</p>
                        <div className="mt-5 flex justify-end gap-2">
                            <button onClick={() => setConfirming(false)} disabled={saving} className="rounded-md border border-slate-700 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-800">
                                Cancel
                            </button>
                            <button onClick={doSave} disabled={saving} className="inline-flex items-center gap-1.5 rounded-md bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-500 disabled:opacity-50">
                                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                                Save changes
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}
        </div>
    );
}
