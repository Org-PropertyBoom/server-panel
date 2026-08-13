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
    FilePlus,
    Pencil,
    Trash2,
    X,
    FolderOpen,
} from "lucide-react";
import { toast } from "sonner";

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

// Editor session, persisted locally so a reload (or a panel update) reopens what you
// were working on. Scoped per mode so a root and a user session don't share tabs.
const OPEN_TABS_KEY = `files_open_tabs_${runtime.isRoot ? "root" : "user"}`;
const ACTIVE_FILE_KEY = `files_active_file_${runtime.isRoot ? "root" : "user"}`;

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
    const [revealTarget, setRevealTarget] = useState("");

    // New file/folder modal
    const [createOpen, setCreateOpen] = useState(false);
    const [createDir, setCreateDir] = useState("/");

    // Open editor tabs, restored across reloads so a refresh (or a panel update)
    // doesn't lose your place. Only the paths are persisted — contents are re-read
    // from disk, which is also what keeps a tab honest if the file changed.
    const [openTabs, setOpenTabs] = useState<{ name: string; path: string }[]>(() => {
        try {
            const raw = window.localStorage.getItem(OPEN_TABS_KEY);
            const parsed = raw ? JSON.parse(raw) : [];
            return Array.isArray(parsed) ? parsed.filter((t) => t && typeof t.path === "string") : [];
        } catch {
            return [];
        }
    });

    useEffect(() => {
        try {
            window.localStorage.setItem(OPEN_TABS_KEY, JSON.stringify(openTabs));
        } catch {
            /* private mode / quota — tabs just won't persist */
        }
    }, [openTabs]);

    // Captured during the FIRST RENDER, before any effect runs. The effect below
    // fires on mount with selectedFile still null and clears the stored path, so
    // reading it later (from the restore effect) would always come back empty.
    const restoreTarget = useRef<string>(
        (() => {
            try {
                return window.localStorage.getItem(ACTIVE_FILE_KEY) ?? "";
            } catch {
                return "";
            }
        })(),
    );

    useEffect(() => {
        try {
            if (selectedFile) window.localStorage.setItem(ACTIVE_FILE_KEY, selectedFile.path);
            else window.localStorage.removeItem(ACTIVE_FILE_KEY);
        } catch {
            /* ignore */
        }
    }, [selectedFile]);

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
            setOpenTabs((prev) => (prev.some((t) => t.path === item.path) ? prev : [...prev, { name: item.name, path: item.path }]));
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

    // expandPath opens every folder from the root down to targetDir (fetching
    // children as needed). Optionally records a node to scroll into view.
    const expandPath = async (targetDir: string, scrollTo?: string) => {
        const base = homePath || "/";
        if (!targetDir.startsWith(base)) return;
        const ancestors: string[] = [base];
        const rel = targetDir.slice(base === "/" ? 0 : base.length).split("/").filter(Boolean);
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
        if (scrollTo) setRevealTarget(scrollTo);
    };

    // revealInTree expands down to the file's parent so the file node renders +
    // highlights (VS Code "reveal active file"); navigateToFolder jumps to a
    // breadcrumb folder and scrolls to it.
    const revealInTree = (filePath: string) => {
        const i = filePath.lastIndexOf("/");
        return expandPath(i > 0 ? filePath.slice(0, i) : "/");
    };
    const navigateToFolder = (dirPath: string) => expandPath(dirPath, dirPath);

    // Delete a file: remove it, clear the editor if it was open, and refresh its
    // folder in the tree so the node disappears.
    const deleteFile = async (path: string) => {
        const res = await fetch(`${apiEndpoint}?path=${encodeURIComponent(path)}`, { method: "DELETE" });
        if (!res.ok) throw new Error((await res.text()).trim() || "Failed to delete file");
        if (selectedFile?.path === path) setSelectedFile(null);
        setOpenTabs((prev) => prev.filter((t) => t.path !== path)); // a deleted file can't stay open
        const parent = path.slice(0, path.lastIndexOf("/")) || "/";
        const items = await fetchFolderContents(parent);
        setExpanded((prev) => ({ ...prev, [parent]: items }));
    };

    // Close one tab. If it was the active one, fall to the neighbour (the tab that
    // slid into its place, else the one before) so the editor doesn't go blank while
    // other files are still open.
    const closeTab = (path: string) => {
        const idx = openTabs.findIndex((t) => t.path === path);
        const next = openTabs.filter((t) => t.path !== path);
        setOpenTabs(next);
        if (selectedFile?.path !== path) return;
        const neighbour = next[idx] ?? next[idx - 1];
        if (neighbour) handleSelectNode({ name: neighbour.name, path: neighbour.path, isDir: false, size: 0, modTime: "" });
        else setSelectedFile(null);
    };

    // Create a new file or folder in dir, refresh + expand that folder in the tree,
    // and open the new file in the editor so it's immediately editable.
    const createEntry = async (dir: string, name: string, kind: "file" | "dir") => {
        const res = await fetch(apiEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ dir, name, kind }),
        });
        if (!res.ok) throw new Error((await res.text()).trim() || "Failed to create");
        const items = await fetchFolderContents(dir);
        setExpanded((prev) => ({ ...prev, [dir]: items }));
        if (kind === "file") {
            const path = `${dir === "/" ? "" : dir}/${name}`;
            handleSelectNode({ name, path, isDir: false, size: 0, modTime: "" });
        }
    };

    // Rename a file in place: rename it, re-point the editor if it was open, and
    // refresh its folder in the tree so the node shows the new name.
    const renameFile = async (path: string, newName: string) => {
        const res = await fetch(apiEndpoint, {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path, newName }),
        });
        if (!res.ok) throw new Error((await res.text()).trim() || "Failed to rename file");
        const parent = path.slice(0, path.lastIndexOf("/")) || "/";
        const newPath = `${parent === "/" ? "" : parent}/${newName}`;
        if (selectedFile?.path === path) setSelectedFile({ ...selectedFile, name: newName, path: newPath });
        setOpenTabs((prev) => prev.map((t) => (t.path === path ? { name: newName, path: newPath } : t)));
        const items = await fetchFolderContents(parent);
        setExpanded((prev) => ({ ...prev, [parent]: items }));
    };

    // Open a file directly by path (from quick-search): reveal it in the tree, then load it.
    const openFileByPath = async (path: string, name: string) => {
        await revealInTree(path);
        handleSelectNode({ name, path, isDir: false, size: 0, modTime: "" });
    };

    useEffect(() => {
        // Restore the previously active file after the tree is ready, so it's also
        // revealed in place. A file that's since been deleted surfaces as a normal
        // read error rather than silently vanishing.
        initExplorer().then(() => {
            // Prefer the file that was active; otherwise fall back to the first open
            // tab, so restored tabs are never shown against an empty editor pane.
            const active = restoreTarget.current || openTabs[0]?.path || "";
            if (!active) return;
            openFileByPath(active, active.slice(active.lastIndexOf("/") + 1)).catch(() => undefined);
        });
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
                        <div className="flex items-center gap-0.5">
                            <button
                                onClick={() => {
                                    const parent = selectedFile ? selectedFile.path.slice(0, selectedFile.path.lastIndexOf("/")) || "/" : "/";
                                    setCreateDir(parent);
                                    setCreateOpen(true);
                                }}
                                className="p-1 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                                title="New file or folder"
                            >
                                <FilePlus className="h-3.5 w-3.5" />
                            </button>
                            <button
                                onClick={initExplorer}
                                className="p-1 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                                title="Refresh Explorer"
                                disabled={isLoading}
                            >
                                <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? "animate-spin" : ""}`} />
                            </button>
                        </div>
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
                                revealPath={revealTarget}
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
                            tabs={openTabs}
                            onSelectTab={(path) => {
                                const tab = openTabs.find((t) => t.path === path);
                                if (tab) handleSelectNode({ name: tab.name, path: tab.path, isDir: false, size: 0, modTime: "" });
                            }}
                            onCloseTab={closeTab}
                            canEdit={runtime.isRoot}
                            onSave={selectedFile ? (content) => saveFile(selectedFile.path, content) : undefined}
                            onToggleDetails={selectedFile ? () => setShowDetails((v) => !v) : undefined}
                            detailsOpen={showDetails}
                            onNavigate={navigateToFolder}
                        />
                    </div>
                </div>

                {showDetails && selectedFile && !contentError ? (
                    <FileDetailsPanel file={selectedFile} size={fileSize} isBinary={isBinary} meta={fileMeta} onClose={() => setShowDetails(false)} onDelete={deleteFile} onRename={renameFile} />
                ) : null}
            </div>

            {createOpen ? (
                <CreateEntryModal
                    dir={createDir}
                    onDirChange={setCreateDir}
                    onClose={() => setCreateOpen(false)}
                    onCreate={createEntry}
                />
            ) : null}
        </DashboardLayout>
    );
}

// CreateEntryModal creates a new empty file (or folder) in a chosen directory. The
// folder is pre-filled from the current selection and stays editable, so a file can
// be created anywhere without first navigating there. A new file opens straight in
// the editor, so "create then write" is one flow.
function CreateEntryModal({
    dir,
    onDirChange,
    onClose,
    onCreate,
}: {
    dir: string;
    onDirChange: (dir: string) => void;
    onClose: () => void;
    onCreate: (dir: string, name: string, kind: "file" | "dir") => Promise<void>;
}) {
    const [name, setName] = useState("");
    const [kind, setKind] = useState<"file" | "dir">("file");
    const [busy, setBusy] = useState(false);
    // Synchronous re-entry guard. `disabled={busy}` alone loses the race: setBusy is
    // async, so a fast double-click (or Enter held down) fires submit twice before
    // React re-renders — creating the entry once and erroring the second time.
    const submitting = useRef(false);

    const submit = async () => {
        const trimmed = name.trim();
        if (submitting.current || !trimmed) return;
        submitting.current = true;
        setBusy(true);
        try {
            await onCreate(dir.trim() || "/", trimmed, kind);
            toast.success(`${trimmed} created`);
            onClose();
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Create failed");
        } finally {
            submitting.current = false;
            setBusy(false);
        }
    };

    return (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => (busy ? null : onClose())}>
            <div className="w-full max-w-md rounded-md border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                <h2 className="text-sm font-semibold text-foreground">New {kind === "dir" ? "folder" : "file"}</h2>

                <div className="mt-3 inline-flex rounded-md border border-border p-0.5">
                    {(["file", "dir"] as const).map((k) => (
                        <button
                            key={k}
                            onClick={() => setKind(k)}
                            className={`rounded px-2.5 py-1 text-xs font-medium ${kind === k ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"}`}
                        >
                            {k === "file" ? "File" : "Folder"}
                        </button>
                    ))}
                </div>

                <label className="mt-3 block">
                    <span className="mb-1 block text-[11px] uppercase tracking-wide text-muted-foreground">In folder</span>
                    <input
                        value={dir}
                        onChange={(e) => onDirChange(e.target.value)}
                        spellCheck={false}
                        className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs outline-none focus:border-primary"
                    />
                </label>

                <label className="mt-3 block">
                    <span className="mb-1 block text-[11px] uppercase tracking-wide text-muted-foreground">Name</span>
                    <input
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") submit();
                            if (e.key === "Escape") onClose();
                        }}
                        placeholder={kind === "dir" ? "my-folder" : "notes.txt"}
                        autoFocus
                        spellCheck={false}
                        className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm outline-none focus:border-primary"
                    />
                </label>
                <p className="mt-1.5 text-[11px] text-muted-foreground">
                    {kind === "dir" ? "Creates an empty folder." : "Creates an empty file and opens it in the editor."} Won't overwrite an existing entry.
                </p>

                <div className="mt-5 flex justify-end gap-2">
                    <button onClick={onClose} disabled={busy} className="rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted">
                        Cancel
                    </button>
                    <button
                        onClick={submit}
                        disabled={busy || !name.trim()}
                        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
                    >
                        {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <FilePlus className="h-4 w-4" />}
                        Create
                    </button>
                </div>
            </div>
        </div>
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
function FileDetailsPanel({ file, size, isBinary, meta, onClose, onDelete, onRename }: { file: FileItem; size: number; isBinary: boolean; meta: FileMeta | null; onClose: () => void; onDelete: (path: string) => Promise<void>; onRename: (path: string, newName: string) => Promise<void> }) {
    const [confirming, setConfirming] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const [renaming, setRenaming] = useState(false);
    const [renameTo, setRenameTo] = useState("");
    const [renameBusy, setRenameBusy] = useState(false);
    // Synchronous guards — see CreateEntryModal: the disabled prop can't stop a
    // second click that lands before React re-renders.
    const renameRunning = useRef(false);
    const deleteRunning = useRef(false);
    const doRename = async () => {
        const name = renameTo.trim();
        if (renameRunning.current) return;
        if (!name || name === file.name) {
            setRenaming(false);
            return;
        }
        renameRunning.current = true;
        setRenameBusy(true);
        try {
            await onRename(file.path, name);
            toast.success(`Renamed to ${name}`);
            setRenaming(false);
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Rename failed");
        } finally {
            renameRunning.current = false;
            setRenameBusy(false);
        }
    };
    const doDelete = async () => {
        if (deleteRunning.current) return;
        deleteRunning.current = true;
        setDeleting(true);
        try {
            await onDelete(file.path);
            toast.success(`${file.name} deleted`);
            setConfirming(false);
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Delete failed");
        } finally {
            deleteRunning.current = false;
            setDeleting(false);
        }
    };
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
            <div className="space-y-2 border-t border-border p-3">
                <button
                    onClick={() => {
                        setRenameTo(file.name);
                        setRenaming(true);
                    }}
                    className="flex w-full items-center justify-center gap-2 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground hover:bg-muted"
                >
                    <Pencil className="h-3.5 w-3.5" /> Rename
                </button>
                <button
                    onClick={() => setConfirming(true)}
                    className="flex w-full items-center justify-center gap-2 rounded-md border border-destructive/30 px-3 py-1.5 text-xs font-medium text-destructive hover:bg-destructive/10"
                >
                    <Trash2 className="h-3.5 w-3.5" /> Delete file
                </button>
            </div>

            {renaming ? (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => (renameBusy ? null : setRenaming(false))}>
                    <div className="w-full max-w-md rounded-md border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                        <h2 className="text-sm font-semibold text-foreground">Rename file</h2>
                        <p className="mt-2 break-all font-mono text-[11px] text-muted-foreground">{file.path}</p>
                        <input
                            value={renameTo}
                            onChange={(e) => setRenameTo(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === "Enter") doRename();
                                if (e.key === "Escape") setRenaming(false);
                            }}
                            autoFocus
                            spellCheck={false}
                            className="mt-3 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm outline-none focus:border-primary"
                        />
                        <p className="mt-1.5 text-[11px] text-muted-foreground">Stays in the same folder — a name, not a path.</p>
                        <div className="mt-5 flex justify-end gap-2">
                            <button onClick={() => setRenaming(false)} disabled={renameBusy} className="rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted">
                                Cancel
                            </button>
                            <button
                                onClick={doRename}
                                disabled={renameBusy || !renameTo.trim() || renameTo.trim() === file.name}
                                className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
                            >
                                {renameBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Pencil className="h-4 w-4" />}
                                Rename
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}

            {confirming ? (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => (deleting ? null : setConfirming(false))}>
                    <div className="w-full max-w-md rounded-md border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                        <h2 className="text-sm font-semibold text-foreground">Delete this file?</h2>
                        <p className="mt-2 text-xs text-muted-foreground">
                            Permanently deletes <b className="text-foreground">{file.name}</b>. This can't be undone.
                        </p>
                        <p className="mt-2 break-all font-mono text-[11px] text-muted-foreground">{file.path}</p>
                        <div className="mt-5 flex justify-end gap-2">
                            <button onClick={() => setConfirming(false)} disabled={deleting} className="rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted">
                                Cancel
                            </button>
                            <button onClick={doDelete} disabled={deleting} className="inline-flex items-center gap-1.5 rounded-md bg-destructive px-3 py-1.5 text-xs font-medium text-destructive-foreground hover:opacity-90 disabled:opacity-50">
                                {deleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                                Delete
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}
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
    revealPath: string;
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
    revealPath,
}: DirectoryTreeNodeProps) {
    const isExpanded = openPaths[path] || false;
    const isSelected = selectedPath === path;
    const children = expanded[path] || [];
    const nodeRef = useRef<HTMLDivElement>(null);

    // Scroll into view when this node is the active file or a breadcrumb reveal target.
    useEffect(() => {
        if (isSelected || path === revealPath) nodeRef.current?.scrollIntoView({ block: "nearest" });
    }, [isSelected, revealPath, path]);

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
                className={`flex items-center gap-1.5 py-1 pr-2 rounded-md cursor-pointer hover:bg-muted/60 transition-colors text-xs ${
                    isSelected ? "bg-primary/10 text-primary font-medium" : "text-foreground/90"
                }`}
                // pr-2 only: the left inset is the depth indent below. Using px-2 here
                // set a padding-left that the inline style then had to override — two
                // sources for one value.
                style={{ paddingLeft: `${depth * 10 + 8}px` }}
                onClick={handleClick}
            >
                {/*
                  * Twistie slot — the VS Code model: a fixed-size box that is ALWAYS
                  * present, carrying a chevron for directories and nothing for files.
                  * Because both branches render the same 16x16 box, a file's icon and
                  * name land on exactly the same x as a sibling folder's.
                  *
                  * The size is stated explicitly (h-4 w-4 + centering) rather than
                  * left to padding around the glyph. The previous version sized the
                  * directory branch as a <button> with p-0.5 around a 12px icon and
                  * the file branch as a hard-coded w-4: two different ways of arriving
                  * at "16px", which only agree while the button's box model does. Any
                  * UA button styling, a border, or a line-height difference desynced
                  * them, and every file in the tree then sat a few px right of its
                  * folder siblings.
                  */}
                {isDir ? (
                    <button
                        onClick={handleToggle}
                        aria-label={isExpanded ? `Collapse ${name}` : `Expand ${name}`}
                        className="flex h-4 w-4 shrink-0 items-center justify-center rounded border-0 p-0 leading-none text-muted-foreground hover:bg-muted-foreground/10"
                    >
                        {isExpanded ? (
                            <ChevronDown className="h-3 w-3" />
                        ) : (
                            <ChevronRight className="h-3 w-3" />
                        )}
                    </button>
                ) : (
                    <span aria-hidden className="block h-4 w-4 shrink-0" />
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
                            revealPath={revealPath}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
