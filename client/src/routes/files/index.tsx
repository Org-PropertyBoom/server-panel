import { useEffect, useRef, useState } from "react";
import {
    Folder,
    File,
    ChevronRight,
    ChevronDown,
    FileText,
    Image,
    Music,
    Video,
    Archive,
    Home,
    Loader2,
    AlertCircle,
    RefreshCw,
    Search,
    X,
    FolderOpen,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import FileEditor from "../../_components/file-editor";
import { runtime } from "../../runtime";

interface FileItem {
    name: string;
    isDir: boolean;
    size: number;
    modTime: string;
    path: string;
}

interface DirectoryList {
    currentPath: string;
    parentPath: string;
    items: FileItem[];
}

const apiEndpoint = runtime.isRoot ? "/post/files" : "/api/files";

export default function FilesRoute() {
    const [homePath, setHomePath] = useState<string>("");
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Tree Explorer state
    const [expanded, setExpanded] = useState<Record<string, FileItem[]>>({});
    const [openPaths, setOpenPaths] = useState<Record<string, boolean>>({});

    // Active File Viewer state
    const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);
    const [fileContent, setFileContent] = useState<string>("");
    const [isBinary, setIsBinary] = useState(false);
    const [fileSize, setFileSize] = useState<number>(0);
    const [isContentLoading, setIsContentLoading] = useState(false);
    const [contentError, setContentError] = useState<string | null>(null);
    const [fileMeta, setFileMeta] = useState<FileMeta | null>(null);
    const [showDetails, setShowDetails] = useState(true);

    // Initialize root / home directory
    const initExplorer = async () => {
        setIsLoading(true);
        setError(null);
        try {
            const initialPath = new URLSearchParams(window.location.search).get("path") ?? "";
            const response = await fetch(`${apiEndpoint}?path=${encodeURIComponent(initialPath)}`);
            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || "Failed to initialize root path");
            }
            const data: DirectoryList = await response.json();
            setHomePath(data.currentPath);

            // Fetch and set items for the root folder
            const items = await fetchFolderContents(data.currentPath);
            setExpanded((prev) => ({ ...prev, [data.currentPath]: items }));
            setOpenPaths((prev) => ({ ...prev, [data.currentPath]: true }));
        } catch (err: any) {
            setError(err.message || "Could not load file system.");
        } finally {
            setIsLoading(false);
        }
    };

    // Load folder contents (directories & files)
    const fetchFolderContents = async (path: string): Promise<FileItem[]> => {
        try {
            const response = await fetch(`${apiEndpoint}?path=${encodeURIComponent(path)}`);
            if (!response.ok) return [];
            const data: DirectoryList = await response.json();
            
            // Sort: directories first, then files
            return (data.items || []).sort((a, b) => {
                if (a.isDir && !b.isDir) return -1;
                if (!a.isDir && b.isDir) return 1;
                return a.name.localeCompare(b.name);
            });
        } catch {
            return [];
        }
    };

    const handleToggleExpand = async (path: string) => {
        const isOpen = openPaths[path] || false;
        
        if (!isOpen) {
            if (!expanded[path]) {
                const items = await fetchFolderContents(path);
                setExpanded((prev) => ({ ...prev, [path]: items }));
            }
            setOpenPaths((prev) => ({ ...prev, [path]: true }));
        } else {
            setOpenPaths((prev) => ({ ...prev, [path]: false }));
        }
    };

    const handleSelectNode = async (item: FileItem) => {
        if (item.isDir) {
            await handleToggleExpand(item.path);
        } else {
            // Load file content
            setSelectedFile(item);
            setIsContentLoading(true);
            setContentError(null);
            try {
                const response = await fetch(`${apiEndpoint}?path=${encodeURIComponent(item.path)}&content=true`);
                if (!response.ok) {
                    const text = await response.text();
                    throw new Error(text || "Failed to load file contents");
                }
                const data = await response.json();
                setFileContent(data.content || "");
                setIsBinary(data.isBinary || false);
                setFileSize(data.size || 0);
                setFileMeta({ modified: data.modified, mode: data.mode, owner: data.owner, group: data.group, lines: data.lines });
            } catch (err: any) {
                setContentError(err.message || "Failed to read file");
            } finally {
                setIsContentLoading(false);
            }
        }
    };

    // Save edits back to the file (root only). Backend backs up + writes atomically.
    const saveFile = async (path: string, content: string) => {
        const response = await fetch(apiEndpoint, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path, content }),
        });
        if (!response.ok) throw new Error((await response.text()).trim() || "Failed to save file");
        setFileContent(content);
        setFileSize(new Blob([content]).size);
        setFileMeta((m) => (m ? { ...m, modified: new Date().toISOString(), lines: content ? content.split("\n").length : 0 } : m));
    };

    // revealInTree expands every ancestor folder from the root down to the file's
    // parent (fetching children as needed) so the file node renders + highlights —
    // the VS Code "reveal active file" behavior, needed when opening via search.
    const revealInTree = async (filePath: string) => {
        const lastSlash = filePath.lastIndexOf("/");
        const parent = lastSlash > 0 ? filePath.slice(0, lastSlash) : "/";
        const base = homePath || "/";
        if (!parent.startsWith(base)) return;
        const ancestors: string[] = [base];
        const rel = parent.slice(base === "/" ? 0 : base.length).split("/").filter(Boolean);
        let cur = base === "/" ? "" : base;
        for (const seg of rel) {
            cur = `${cur}/${seg}`;
            ancestors.push(cur);
        }
        const nextExpanded = { ...expanded };
        const nextOpen = { ...openPaths };
        for (const dir of ancestors) {
            if (!nextExpanded[dir]) {
                nextExpanded[dir] = await fetchFolderContents(dir);
            }
            nextOpen[dir] = true;
        }
        setExpanded(nextExpanded);
        setOpenPaths(nextOpen);
    };

    // Open a file directly by path (from quick-search): reveal it in the tree, then load it.
    const openFileByPath = async (path: string, name: string) => {
        await revealInTree(path);
        handleSelectNode({ name, path, isDir: false, size: 0, modTime: "" });
    };

    useEffect(() => {
        initExplorer();
    }, []);

    const formatBytes = (bytes: number) => {
        if (bytes === 0) return "0 Bytes";
        const k = 1024;
        const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
    };

    return (
        <DashboardLayout
            title="Files"
            description="Manage and edit configuration files exactly like VSCode."
            fullWidth={true}
        >
            <div className={`grid grid-cols-1 overflow-hidden h-full w-full bg-background ${showDetails && selectedFile ? "md:grid-cols-[280px_1fr_280px]" : "md:grid-cols-[280px_1fr]"}`}>
                {/* 1. Left Explorer Sidebar (VSCode Explorer Style) */}
                <aside className="border-r border-border bg-card/60 flex flex-col h-full overflow-hidden select-none">
                    <div className="flex h-10 items-center justify-between px-3 border-b border-border bg-muted/20">
                        <span className="text-xs font-semibold text-muted-foreground">
                            Explorer
                        </span>
                        <button
                            onClick={initExplorer}
                            className="p-1 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                            title="Refresh Explorer"
                            disabled={isLoading}
                        >
                            <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? "animate-spin" : ""}`} />
                        </button>
                    </div>

                    <div className="flex-1 overflow-y-auto py-2 px-2">
                        {isLoading ? (
                            <div className="flex items-center justify-center py-12">
                                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                            </div>
                        ) : error ? (
                            <div className="text-xs text-destructive p-3 text-center">
                                <AlertCircle className="h-5 w-5 mx-auto mb-2 text-destructive" />
                                <span>{error}</span>
                            </div>
                        ) : homePath ? (
                            <DirectoryTreeNode
                                path={homePath}
                                name={runtime.isRoot ? "/" : runtime.username}
                                isDir={true}
                                depth={0}
                                selectedPath={selectedFile?.path || ""}
                                onSelect={handleSelectNode}
                                expanded={expanded}
                                openPaths={openPaths}
                                onToggle={handleToggleExpand}
                            />
                        ) : null}
                    </div>
                </aside>

                {/* 2. Center: quick-search bar + editor pane (VSCode Tab/Editor Style) */}
                <div className="flex flex-col overflow-hidden min-w-0">
                    <FileSearch onOpen={openFileByPath} />
                    <div className="flex-1 min-h-0">
                        <FileEditor
                            fileName={selectedFile?.name || ""}
                            filePath={selectedFile?.path || ""}
                            fileSize={fileSize}
                            content={fileContent}
                            isBinary={isBinary}
                            isLoading={isContentLoading}
                            error={contentError}
                            onClose={() => setSelectedFile(null)}
                            canEdit={runtime.isRoot}
                            onSave={selectedFile ? (content) => saveFile(selectedFile.path, content) : undefined}
                            onToggleDetails={selectedFile ? () => setShowDetails((v) => !v) : undefined}
                            detailsOpen={showDetails}
                        />
                    </div>
                </div>

                {showDetails && selectedFile && !contentError ? (
                    <FileDetailsPanel file={selectedFile} size={fileSize} isBinary={isBinary} meta={fileMeta} onClose={() => setShowDetails(false)} />
                ) : null}
            </div>
        </DashboardLayout>
    );
}

function parentDir(path: string): string {
    const i = path.lastIndexOf("/");
    return i <= 0 ? "/" : path.slice(0, i);
}

// FileSearch is the VS Code-style quick-open: type a name, get ranked suggestions
// from a bounded server-side walk; ↑/↓ + Enter (or click) opens. Ctrl/Cmd+P focuses.
function FileSearch({ onOpen }: { onOpen: (path: string, name: string) => void }) {
    const [q, setQ] = useState("");
    const [results, setResults] = useState<FileItem[]>([]);
    const [open, setOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const [active, setActive] = useState(0);
    const inputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "p") {
                e.preventDefault();
                inputRef.current?.focus();
                inputRef.current?.select();
            }
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);

    useEffect(() => {
        const query = q.trim();
        if (query.length < 2) {
            setResults([]);
            setOpen(false);
            return;
        }
        setLoading(true);
        const t = window.setTimeout(async () => {
            try {
                const res = await fetch(`${apiEndpoint}?q=${encodeURIComponent(query)}`, { cache: "no-store" });
                const data = await res.json();
                setResults(data.items || []);
                setActive(0);
                setOpen(true);
            } catch {
                setResults([]);
            } finally {
                setLoading(false);
            }
        }, 250);
        return () => window.clearTimeout(t);
    }, [q]);

    const choose = (item: FileItem) => {
        onOpen(item.path, item.name);
        setQ("");
        setResults([]);
        setOpen(false);
    };

    const onKeyDown = (e: React.KeyboardEvent) => {
        if (!open || results.length === 0) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            setActive((a) => Math.min(a + 1, results.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setActive((a) => Math.max(a - 1, 0));
        } else if (e.key === "Enter") {
            e.preventDefault();
            if (results[active]) choose(results[active]);
        } else if (e.key === "Escape") {
            setOpen(false);
        }
    };

    return (
        <div className="relative shrink-0 border-b border-border bg-card/40 px-3 py-2">
            <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <input
                    ref={inputRef}
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    onKeyDown={onKeyDown}
                    onFocus={() => results.length > 0 && setOpen(true)}
                    onBlur={() => window.setTimeout(() => setOpen(false), 150)}
                    placeholder="Search files by name…   Ctrl+P"
                    className="w-full rounded-md border border-border bg-background py-1.5 pl-8 pr-8 text-xs outline-none focus:border-primary"
                    spellCheck={false}
                    autoComplete="off"
                />
                {loading ? <Loader2 className="absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin text-muted-foreground" /> : null}
            </div>
            {open && results.length > 0 ? (
                <ul className="absolute left-3 right-3 z-20 mt-1 max-h-80 overflow-auto rounded-md border border-border bg-card shadow-xl">
                    {results.map((r, i) => (
                        <li key={r.path}>
                            <button
                                type="button"
                                onMouseDown={(e) => { e.preventDefault(); choose(r); }}
                                onMouseEnter={() => setActive(i)}
                                className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs ${i === active ? "bg-primary/10" : "hover:bg-muted/60"}`}
                            >
                                <FileText className="h-3.5 w-3.5 shrink-0 text-slate-400" />
                                <span className="shrink-0 font-medium text-foreground">{r.name}</span>
                                <span className="truncate text-muted-foreground" title={r.path}>{parentDir(r.path)}</span>
                            </button>
                        </li>
                    ))}
                </ul>
            ) : open && q.trim().length >= 2 && !loading ? (
                <div className="absolute left-3 right-3 z-20 mt-1 rounded-md border border-border bg-card px-3 py-2 text-xs text-muted-foreground shadow-xl">
                    No files matching “{q.trim()}” — system dirs like /usr, /proc are skipped for speed.
                </div>
            ) : null}
        </div>
    );
}

interface FileMeta {
    modified?: string;
    mode?: string;
    owner?: string;
    group?: string;
    lines?: number;
}

function fmtFileBytes(bytes: number) {
    if (bytes === 0) return "0 Bytes";
    const k = 1024;
    const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function relativeTime(iso?: string): string | undefined {
    if (!iso) return undefined;
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return undefined;
    const s = Math.round((Date.now() - then) / 1000);
    if (s < 5) return "just now";
    if (s < 60) return `${s}s ago`;
    const m = Math.round(s / 60);
    if (m < 60) return `${m}m ago`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.round(h / 24);
    if (d < 30) return `${d}d ago`;
    return new Date(iso).toLocaleDateString();
}

// FileDetailsPanel is the right-hand metadata pane (VS Code-style): type, size,
// timestamps, permissions, owner, line count, and the full path.
function FileDetailsPanel({ file, size, isBinary, meta, onClose }: { file: FileItem; size: number; isBinary: boolean; meta: FileMeta | null; onClose: () => void }) {
    const rows: { label: string; value?: string; sub?: string; mono?: boolean }[] = [
        { label: "Type", value: isBinary ? "Binary" : "Text file" },
        { label: "Size", value: fmtFileBytes(size), sub: `${size.toLocaleString()} bytes` },
        { label: "Modified", value: relativeTime(meta?.modified), sub: meta?.modified ? new Date(meta.modified).toLocaleString() : undefined },
        { label: "Permissions", value: meta?.mode, mono: true },
        { label: "Owner", value: meta?.owner ? `${meta.owner}:${meta.group ?? ""}` : undefined, mono: true },
        { label: "Lines", value: !isBinary && meta?.lines ? meta.lines.toLocaleString() : undefined },
    ];
    return (
        <aside className="border-l border-border bg-card/60 flex flex-col h-full overflow-hidden select-none">
            <div className="flex h-10 items-center justify-between px-3 border-b border-border bg-muted/20">
                <span className="text-xs font-semibold text-muted-foreground">Details</span>
                <button onClick={onClose} title="Hide details" className="p-1 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors">
                    <X className="h-3.5 w-3.5" />
                </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 text-xs">
                <div className="mb-4 flex items-center gap-2">
                    <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium text-foreground" title={file.name}>{file.name}</span>
                </div>
                <dl className="space-y-3">
                    {rows.map((r) =>
                        r.value ? (
                            <div key={r.label}>
                                <dt className="text-[11px] uppercase tracking-wide text-muted-foreground">{r.label}</dt>
                                <dd className={`mt-0.5 text-foreground ${r.mono ? "font-mono text-[11px]" : ""}`}>
                                    {r.value}
                                    {r.sub ? <span className="ml-1.5 text-muted-foreground">· {r.sub}</span> : null}
                                </dd>
                            </div>
                        ) : null,
                    )}
                    <div>
                        <dt className="text-[11px] uppercase tracking-wide text-muted-foreground">Path</dt>
                        <dd className="mt-0.5 break-all font-mono text-[11px] text-foreground">{file.path}</dd>
                    </div>
                </dl>
            </div>
        </aside>
    );
}

// Tree view Node component helper (directories & files mixed)
interface DirectoryTreeNodeProps {
    path: string;
    name: string;
    isDir: boolean;
    depth: number;
    selectedPath: string;
    onSelect: (item: FileItem) => void;
    expanded: Record<string, FileItem[]>;
    openPaths: Record<string, boolean>;
    onToggle: (path: string) => Promise<void>;
}

function DirectoryTreeNode({
    path,
    name,
    isDir,
    depth,
    selectedPath,
    onSelect,
    expanded,
    openPaths,
    onToggle,
}: DirectoryTreeNodeProps) {
    const isExpanded = openPaths[path] || false;
    const isSelected = selectedPath === path;
    const children = expanded[path] || [];
    const nodeRef = useRef<HTMLDivElement>(null);

    // Scroll the active file into view when it's revealed/selected (VS Code-like).
    useEffect(() => {
        if (isSelected) nodeRef.current?.scrollIntoView({ block: "nearest" });
    }, [isSelected]);

    const handleToggle = async (e: React.MouseEvent) => {
        e.stopPropagation();
        if (isDir) {
            await onToggle(path);
        }
    };

    const handleClick = () => {
        onSelect({ name, isDir, path, size: 0, modTime: "" });
    };

    return (
        <div className="select-none">
            <div
                ref={nodeRef}
                className={`flex items-center gap-1.5 py-1 px-2 rounded-md cursor-pointer hover:bg-muted/60 transition-colors text-xs ${
                    isSelected ? "bg-primary/10 text-primary font-medium" : "text-foreground/90"
                }`}
                style={{ paddingLeft: `${depth * 10 + 8}px` }}
                onClick={handleClick}
            >
                {isDir ? (
                    <button
                        onClick={handleToggle}
                        className="p-0.5 rounded hover:bg-muted-foreground/10 text-muted-foreground shrink-0"
                    >
                        {isExpanded ? (
                            <ChevronDown className="h-3 w-3" />
                        ) : (
                            <ChevronRight className="h-3 w-3" />
                        )}
                    </button>
                ) : (
                    // File spacer to align with folders
                    <div className="w-4 h-4 shrink-0" />
                )}
                {isDir ? (
                    <Folder className={`h-3.5 w-3.5 shrink-0 ${isSelected ? "text-primary fill-primary/10" : "text-muted-foreground"}`} />
                ) : (
                    <FileText className="h-3.5 w-3.5 shrink-0 text-slate-400" />
                )}
                <span className="truncate flex-1 min-w-0">{name}</span>
            </div>

            {isDir && isExpanded && children.length > 0 && (
                <div className="mt-0.5">
                    {children.map((item) => (
                        <DirectoryTreeNode
                            key={item.path}
                            path={item.path}
                            name={item.name}
                            isDir={item.isDir}
                            depth={depth + 1}
                            selectedPath={selectedPath}
                            onSelect={onSelect}
                            expanded={expanded}
                            openPaths={openPaths}
                            onToggle={onToggle}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
