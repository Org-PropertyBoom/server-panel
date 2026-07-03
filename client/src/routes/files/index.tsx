import { useEffect, useState } from "react";
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
    ArrowUp,
    AlertCircle,
    RefreshCw,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
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
    const [currentPath, setCurrentPath] = useState<string>("");
    const [parentPath, setParentPath] = useState<string>("");
    const [files, setFiles] = useState<FileItem[]>([]);
    const [homePath, setHomePath] = useState<string>("");
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Tree state
    const [expanded, setExpanded] = useState<Record<string, { name: string; path: string }[]>>({});
    const [openPaths, setOpenPaths] = useState<Record<string, boolean>>({});

    const fetchDirectory = async (path: string, selectInTree = true) => {
        setIsLoading(true);
        setError(null);
        try {
            const response = await fetch(`${apiEndpoint}?path=${encodeURIComponent(path)}`);
            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || "Failed to load directory");
            }
            const data: DirectoryList = await response.json();
            
            // Sort: folders first, then files
            const sortedItems = (data.items || []).sort((a, b) => {
                if (a.isDir && !b.isDir) return -1;
                if (!a.isDir && b.isDir) return 1;
                return a.name.localeCompare(b.name);
            });

            setFiles(sortedItems);
            setCurrentPath(data.currentPath);
            setParentPath(data.parentPath);

            // Set homePath on first load
            if (!homePath) {
                setHomePath(data.currentPath);
            }

            // Auto-expand and load parent folders in tree if selected from table
            if (selectInTree && data.currentPath) {
                await expandPathInTree(data.currentPath);
            }
        } catch (err: any) {
            setError(err.message || "An error occurred while reading the files.");
        } finally {
            setIsLoading(false);
        }
    };

    // Load subfolders for the tree view
    const fetchSubfoldersOnly = async (path: string): Promise<{ name: string; path: string }[]> => {
        try {
            const response = await fetch(`${apiEndpoint}?path=${encodeURIComponent(path)}`);
            if (!response.ok) return [];
            const data: DirectoryList = await response.json();
            return (data.items || [])
                .filter((item) => item.isDir)
                .map((item) => ({ name: item.name, path: item.path }))
                .sort((a, b) => a.name.localeCompare(b.name));
        } catch {
            return [];
        }
    };

    const handleToggleExpand = async (path: string) => {
        const isOpen = openPaths[path] || false;
        
        if (!isOpen) {
            // Lazy load if not already loaded
            if (!expanded[path]) {
                const subdirs = await fetchSubfoldersOnly(path);
                setExpanded((prev) => ({ ...prev, [path]: subdirs }));
            }
            setOpenPaths((prev) => ({ ...prev, [path]: true }));
        } else {
            setOpenPaths((prev) => ({ ...prev, [path]: false }));
        }
    };

    // Helper to auto-expand parent paths down to the selected path
    const expandPathInTree = async (targetPath: string) => {
        if (!homePath) return;

        // Find segments between homePath and targetPath
        let current = targetPath;
        const pathsToExpand: string[] = [];

        while (current && current.length >= homePath.length) {
            pathsToExpand.unshift(current);
            if (current === homePath) break;
            const parent = current.substring(0, current.lastIndexOf("/"));
            current = parent || "/";
        }

        const newExpanded = { ...expanded };
        const newOpenPaths = { ...openPaths };

        for (const p of pathsToExpand) {
            newOpenPaths[p] = true;
            if (!newExpanded[p]) {
                const subdirs = await fetchSubfoldersOnly(p);
                newExpanded[p] = subdirs;
            }
        }

        setExpanded(newExpanded);
        setOpenPaths(newOpenPaths);
    };

    useEffect(() => {
        fetchDirectory("");
    }, []);

    const formatBytes = (bytes: number) => {
        if (bytes === 0) return "-";
        const k = 1024;
        const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
    };

    const getFileIcon = (item: FileItem) => {
        if (item.isDir) return <Folder className="h-5 w-5 text-blue-500 fill-blue-500/10" />;
        
        const ext = item.name.split(".").pop()?.toLowerCase();
        switch (ext) {
            case "txt":
            case "md":
            case "log":
            case "conf":
            case "yaml":
            case "yml":
            case "json":
                return <FileText className="h-5 w-5 text-slate-500" />;
            case "jpg":
            case "jpeg":
            case "png":
            case "gif":
            case "svg":
                return <Image className="h-5 w-5 text-emerald-500" />;
            case "mp3":
            case "wav":
            case "ogg":
                return <Music className="h-5 w-5 text-indigo-500" />;
            case "mp4":
            case "avi":
            case "mkv":
                return <Video className="h-5 w-5 text-red-500" />;
            case "zip":
            case "tar":
            case "gz":
            case "rar":
            case "7z":
                return <Archive className="h-5 w-5 text-amber-500" />;
            default:
                return <File className="h-5 w-5 text-slate-400" />;
        }
    };

    // Build breadcrumbs
    const renderBreadcrumbs = () => {
        if (!currentPath) return null;
        
        const relPath = currentPath.substring(homePath.length);
        const segments = relPath.split("/").filter(Boolean);
        
        return (
            <div className="flex items-center gap-1.5 text-sm text-muted-foreground select-none">
                <button
                    onClick={() => fetchDirectory(homePath)}
                    className="flex items-center gap-1 hover:text-foreground transition-colors"
                >
                    <Home className="h-4 w-4" />
                    <span>Home</span>
                </button>
                {segments.map((seg, idx) => {
                    // Reconstruct full path for this segment
                    const pathUpToSeg = homePath + "/" + segments.slice(0, idx + 1).join("/");
                    return (
                        <div key={pathUpToSeg} className="flex items-center gap-1.5">
                            <span>/</span>
                            <button
                                onClick={() => fetchDirectory(pathUpToSeg)}
                                className="hover:text-foreground transition-colors max-w-[120px] truncate"
                            >
                                {seg}
                            </button>
                        </div>
                    );
                })}
            </div>
        );
    };

    return (
        <DashboardLayout
            title="Files"
            description="Manage your files, documents, and system configuration directories."
            actions={
                <Button
                    variant="outline"
                    className="gap-2"
                    onClick={() => fetchDirectory(currentPath)}
                    disabled={isLoading}
                >
                    <RefreshCw className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
                    Reload
                </Button>
            }
        >
            <div className="grid grid-cols-1 md:grid-cols-[250px_1fr] gap-6 items-start h-[calc(100vh-220px)]">
                {/* 2nd Sidebar (Directory Tree) */}
                <aside className="border border-border rounded-lg bg-card p-4 h-full overflow-y-auto flex flex-col gap-2">
                    <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider px-2 mb-2">
                        Directory Tree
                    </h3>
                    
                    {!homePath ? (
                        <div className="flex justify-center py-8">
                            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                        </div>
                    ) : (
                        <div className="flex-1 overflow-y-auto">
                            <DirectoryTreeNode
                                path={homePath}
                                name="Home"
                                depth={0}
                                selectedPath={currentPath}
                                onSelect={(p) => fetchDirectory(p)}
                                expanded={expanded}
                                openPaths={openPaths}
                                onToggle={handleToggleExpand}
                            />
                        </div>
                    )}
                </aside>

                {/* Main Content Area (File Explorer) */}
                <div className="flex flex-col gap-4 border border-border rounded-lg bg-card p-5 h-full overflow-hidden">
                    {/* Toolbar / Breadcrumbs */}
                    <div className="flex items-center justify-between border-b border-border pb-3">
                        {renderBreadcrumbs()}
                        {parentPath && (
                            <Button
                                size="sm"
                                variant="ghost"
                                className="h-8 gap-1 text-xs text-muted-foreground hover:text-foreground"
                                onClick={() => fetchDirectory(parentPath)}
                            >
                                <ArrowUp className="h-3.5 w-3.5" />
                                Up one level
                            </Button>
                        )}
                    </div>

                    {/* Files List / Loading / Error */}
                    <div className="flex-1 overflow-y-auto min-h-0">
                        {isLoading && files.length === 0 ? (
                            <div className="flex h-full items-center justify-center">
                                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                            </div>
                        ) : error ? (
                            <div className="flex flex-col items-center justify-center h-full gap-3 text-destructive p-6 text-center">
                                <AlertCircle className="h-10 w-10 shrink-0" />
                                <p className="text-sm font-semibold">{error}</p>
                                <Button variant="outline" size="sm" onClick={() => fetchDirectory(currentPath)}>
                                    Try again
                                </Button>
                            </div>
                        ) : (
                            <div className="min-w-full">
                                <div className="grid grid-cols-[1.5fr_100px_160px] border-b border-border bg-muted/20 px-4 py-2 text-xs font-semibold text-muted-foreground rounded-t-md">
                                    <span>Name</span>
                                    <span>Size</span>
                                    <span>Last Modified</span>
                                </div>

                                {files.length === 0 ? (
                                    <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
                                        This directory is empty.
                                    </div>
                                ) : (
                                    <div className="divide-y divide-border">
                                        {files.map((file) => (
                                            <div
                                                key={file.path}
                                                className={`grid grid-cols-[1.5fr_100px_160px] items-center px-4 py-2.5 text-sm hover:bg-muted/40 transition-colors cursor-pointer ${
                                                    file.isDir ? "font-medium" : ""
                                                }`}
                                                onClick={() => {
                                                    if (file.isDir) {
                                                        fetchDirectory(file.path);
                                                    }
                                                }}
                                            >
                                                <div className="flex min-w-0 items-center gap-3">
                                                    {getFileIcon(file)}
                                                    <span className="truncate">{file.name}</span>
                                                </div>
                                                <span className="text-xs text-muted-foreground">
                                                    {formatBytes(file.size)}
                                                </span>
                                                <span className="text-xs text-muted-foreground truncate">
                                                    {new Date(file.modTime).toLocaleString()}
                                                </span>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </DashboardLayout>
    );
}

// Tree view Node component helper
interface DirectoryTreeProps {
    path: string;
    name: string;
    depth: number;
    selectedPath: string;
    onSelect: (path: string) => void;
    expanded: Record<string, { name: string; path: string }[]>;
    openPaths: Record<string, boolean>;
    onToggle: (path: string) => Promise<void>;
}

function DirectoryTreeNode({
    path,
    name,
    depth,
    selectedPath,
    onSelect,
    expanded,
    openPaths,
    onToggle,
}: DirectoryTreeProps) {
    const isExpanded = openPaths[path] || false;
    const isSelected = selectedPath === path;
    const subfolders = expanded[path] || [];

    const handleToggle = async (e: React.MouseEvent) => {
        e.stopPropagation();
        await onToggle(path);
    };

    return (
        <div className="select-none">
            <div
                className={`flex items-center gap-1 py-1 px-2 rounded-md cursor-pointer hover:bg-muted/60 transition-colors text-sm ${
                    isSelected ? "bg-primary/10 text-primary font-medium" : "text-foreground"
                }`}
                style={{ paddingLeft: `${depth * 10 + 8}px` }}
                onClick={() => onSelect(path)}
            >
                <button
                    onClick={handleToggle}
                    className="p-0.5 rounded hover:bg-muted-foreground/10 text-muted-foreground shrink-0"
                >
                    {isExpanded ? (
                        <ChevronDown className="h-3.5 w-3.5" />
                    ) : (
                        <ChevronRight className="h-3.5 w-3.5" />
                    )}
                </button>
                <Folder className={`h-4 w-4 shrink-0 ${isSelected ? "text-primary fill-primary/10" : "text-muted-foreground"}`} />
                <span className="truncate flex-1 min-w-0">{name}</span>
            </div>

            {isExpanded && subfolders.length > 0 && (
                <div className="mt-0.5">
                    {subfolders.map((folder) => (
                        <DirectoryTreeNode
                            key={folder.path}
                            path={folder.path}
                            name={folder.name}
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
