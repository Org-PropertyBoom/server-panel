import {
    FileText,
    X,
    Loader2,
    AlertCircle,
    FolderOpen,
} from "lucide-react";

interface FileEditorProps {
    fileName: string;
    filePath: string;
    fileSize: number;
    content: string;
    isBinary: boolean;
    isLoading: boolean;
    error: string | null;
    onClose?: () => void;
    placeholderTitle?: string;
    placeholderDescription?: string;
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
    placeholderTitle = "Ppt Server Panel Editor",
    placeholderDescription = "Select a configuration file or script from the directory tree sidebar to view or edit its contents.",
}: FileEditorProps) {
    const formatBytes = (bytes: number) => {
        if (bytes === 0) return "0 Bytes";
        const k = 1024;
        const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
    };

    if (!fileName && !filePath) {
        return (
            /* VSCode Empty Welcome Screen */
            <div className="flex-1 flex flex-col items-center justify-center p-8 select-none bg-slate-950 text-slate-500">
                <div className="flex flex-col items-center max-w-md text-center gap-6">
                    <div className="h-16 w-16 items-center justify-center rounded-xl bg-slate-900 border border-slate-800 flex text-slate-400">
                        <FolderOpen className="h-8 w-8" />
                    </div>
                    <div className="space-y-1.5">
                        <h3 className="text-slate-300 font-semibold text-sm">{placeholderTitle}</h3>
                        <p className="text-xs text-slate-600 max-w-xs leading-relaxed">
                            {placeholderDescription}
                        </p>
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
                    {onClose && (
                        <button
                            onClick={onClose}
                            className="ml-2 p-0.5 rounded text-slate-500 hover:bg-slate-800 hover:text-slate-200 transition-colors"
                        >
                            <X className="h-3 w-3" />
                        </button>
                    )}
                </div>
                <div className="flex-1" />
                <div className="px-4 text-[10px] text-slate-500 font-mono">
                    {formatBytes(fileSize)}
                </div>
            </div>

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
                ) : (
                    <div className="flex font-mono text-xs md:text-sm leading-6 overflow-auto h-full bg-slate-950 text-slate-300">
                        {/* Line numbers */}
                        <div className="text-right pr-4 pl-3 select-none text-slate-600 border-r border-slate-800 bg-slate-900/20 sticky left-0 min-w-[3.5rem] py-4 shrink-0">
                            {content.split("\n").map((_, idx) => (
                                <div key={idx} className="h-6">{idx + 1}</div>
                            ))}
                        </div>
                        {/* File body content */}
                        <pre className="pl-4 pr-6 py-4 m-0 select-text whitespace-pre overflow-visible flex-1 leading-6">
                            {content}
                        </pre>
                    </div>
                )}
            </div>

            {/* Editor Status Bar */}
            <div className="border-t border-slate-900 bg-slate-900 px-4 py-1.5 flex items-center justify-between text-[10px] text-slate-500 font-mono select-none shrink-0">
                <span className="truncate max-w-md" title={filePath}>
                    Path: {filePath}
                </span>
                <span>UTF-8</span>
            </div>
        </div>
    );
}
