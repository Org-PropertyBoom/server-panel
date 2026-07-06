import { useCallback, useState, useEffect } from "react";
import ColorModeSwitch from "_components/color-mode-switch";
import { AlertTriangle, User, Menu, RefreshCw, X } from "lucide-react";
import { useApp } from "../../_contexts/app";

type HeaderProps = {
    title: string;
    onMenuClick?: () => void;
};

type UpdateInfo = {
    updateAvailable: boolean;
    localVersion: string;
    remoteVersion: string;
    localBuildTime: string;
    remoteBuildTime: string;
};

export default function Header({ title, onMenuClick }: HeaderProps) {
    const { mode, isRoot } = useApp();
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
    const [checking, setChecking] = useState(false);
    const [updating, setUpdating] = useState(false);
    const [restarting, setRestarting] = useState(false);
    const [bypassBeforeUnload, setBypassBeforeUnload] = useState(false);
    const [updateModalOpen, setUpdateModalOpen] = useState(false);
    const [updateError, setUpdateError] = useState("");
    const [updateSuccess, setUpdateSuccess] = useState(false);

    const checkUpdate = useCallback(async () => {
        if (!isRoot) return null;
        setChecking(true);
        try {
            const response = await fetch("/post/update");
            if (response.ok) {
                const data = await response.json();
                setUpdateAvailable(data.updateAvailable);
                setUpdateInfo(data);
                return data as UpdateInfo;
            }
        } catch (err) {
            console.error("Failed to check for updates", err);
        } finally {
            setChecking(false);
        }

        return null;
    }, [isRoot]);

    useEffect(() => {
        if (isRoot) {
            checkUpdate();
            const interval = setInterval(checkUpdate, 180000);
            return () => clearInterval(interval);
        }
    }, [checkUpdate, isRoot]);

    useEffect(() => {
        if ((!updating && !restarting) || bypassBeforeUnload) return;

        const handleBeforeUnload = (e: BeforeUnloadEvent) => {
            e.preventDefault();
            e.returnValue = "The system is updating or restarting. Please do not close the browser or reload the page.";
            return e.returnValue;
        };

        window.addEventListener("beforeunload", handleBeforeUnload);
        return () => window.removeEventListener("beforeunload", handleBeforeUnload);
    }, [updating, restarting, bypassBeforeUnload]);

    const handleUpdate = async () => {
        setUpdateError("");
        setUpdateSuccess(false);

        if (!updateAvailable) {
            await checkUpdate();
            return;
        }

        setUpdateModalOpen(true);
    };

    const confirmUpdate = async () => {
        setUpdating(true);
        setRestarting(false);
        setUpdateError("");
        setUpdateSuccess(false);
        try {
            const response = await fetch("/post/update", { method: "POST" });
            if (response.ok) {
                setUpdateSuccess(true);
                setRestarting(true);
                setTimeout(() => {
                    setBypassBeforeUnload(true);
                    setTimeout(() => {
                        window.location.reload();
                    }, 50);
                }, 3000);
            } else {
                const msg = await response.text();
                setUpdateError(msg.trim() || "Update failed");
            }
        } catch (err: any) {
            setUpdateError(err.message || "Update failed");
        } finally {
            setUpdating(false);
        }
    };

    return (
        <header className="flex h-14 items-center justify-between border-b border-border bg-card px-6">
            <div className="flex items-center gap-4">
                {onMenuClick && (
                    <button
                        onClick={onMenuClick}
                        className="rounded-md p-1.5 hover:bg-muted md:hidden"
                        aria-label="Toggle menu"
                    >
                        <Menu className="h-5 w-5 text-muted-foreground" />
                    </button>
                )}
                <h1 className="text-sm font-semibold text-foreground md:text-base">
                    {title}
                </h1>
            </div>

            <div className="flex items-center gap-4">
                {isRoot && (
                    <button
                        onClick={handleUpdate}
                        disabled={checking || updating || restarting}
                        className={`relative flex h-8 items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium transition-all ${
                            updateAvailable
                                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/20"
                                : "border-border bg-background hover:bg-muted text-muted-foreground hover:text-foreground"
                        }`}
                        title={updateAvailable ? "New update available!" : "Check for updates"}
                    >
                        <RefreshCw className={`h-3.5 w-3.5 ${checking || updating || restarting ? "animate-spin" : ""}`} />
                        <span>{restarting ? "Restarting..." : updating ? "Updating..." : updateAvailable ? "Update Available" : "Check Update"}</span>
                        {updateAvailable && (
                            <span className="absolute -right-1 -top-1 flex h-2.5 w-2.5">
                                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
                                <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500"></span>
                            </span>
                        )}
                    </button>
                )}

                <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1 text-xs text-muted-foreground">
                    <User className="h-3 w-3" />
                    <span className="capitalize">{mode} session</span>
                </div>
                <ColorModeSwitch />
            </div>

            {updateModalOpen && (
                <UpdateModal
                    checking={checking}
                    error={updateError}
                    info={updateInfo}
                    success={updateSuccess}
                    updating={updating}
                    restarting={restarting}
                    onCheck={checkUpdate}
                    onClose={() => {
                        if (!updating && !restarting) {
                            setUpdateModalOpen(false);
                        }
                    }}
                    onConfirm={confirmUpdate}
                />
            )}
        </header>
    );
}

function UpdateModal({
    checking,
    error,
    info,
    success,
    updating,
    restarting,
    onCheck,
    onClose,
    onConfirm,
}: {
    checking: boolean;
    error: string;
    info: UpdateInfo | null;
    success: boolean;
    updating: boolean;
    restarting: boolean;
    onCheck: () => Promise<UpdateInfo | null>;
    onClose: () => void;
    onConfirm: () => void;
}) {
    const localVersion = displayVersion(info?.localVersion, info?.localBuildTime);
    const remoteVersion = displayVersion(info?.remoteVersion, info?.remoteBuildTime);

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 backdrop-blur-sm">
            <div className="w-full max-w-lg rounded-md border border-border bg-card text-card-foreground shadow-lg">
                <div className="flex items-center justify-between border-b border-border px-5 py-4">
                    <div className="flex items-center gap-3">
                        <span className="flex h-9 w-9 items-center justify-center rounded-md border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                            <RefreshCw className="h-4 w-4" />
                        </span>
                        <div>
                            <h2 className="text-sm font-semibold">Update server</h2>
                            <p className="text-xs text-muted-foreground">
                                Review the build before restarting the service.
                            </p>
                        </div>
                    </div>
                    <button
                        className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                        disabled={updating || restarting}
                        onClick={onClose}
                        title="Close"
                        type="button"
                    >
                        <X className="h-4 w-4" />
                    </button>
                </div>

                <div className="space-y-4 px-5 py-5">
                    <div className="grid gap-3 sm:grid-cols-2">
                        <VersionBlock
                            buildTime={info?.localBuildTime}
                            label="Current"
                            version={localVersion}
                        />
                        <VersionBlock
                            buildTime={info?.remoteBuildTime}
                            label="Available"
                            version={remoteVersion}
                        />
                    </div>

                    <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                        The server will download the new binary, replace the current executable, and restart.
                    </div>

                    {(updating || restarting) && (
                        <div className="flex gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-xs text-amber-700 dark:text-amber-300 animate-pulse">
                            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
                            <div className="space-y-1">
                                <p className="font-semibold text-amber-800 dark:text-amber-200">
                                    {restarting ? "System is restarting..." : "System is updating..."}
                                </p>
                                <p className="leading-relaxed">
                                    {restarting
                                        ? "The system is restarting to apply the update. Please do not close the browser, close the tab, or reload this page."
                                        : "Please do not close the browser, close the tab, or reload this page until the process is complete."}
                                </p>
                            </div>
                        </div>
                    )}

                    {error && (
                        <div className="flex gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                            <span>{error}</span>
                        </div>
                    )}

                    {success && (
                        <div className="rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
                            Update installed. The server is restarting; the page will reload shortly.
                        </div>
                    )}
                </div>

                <div className="flex items-center justify-end gap-2 border-t border-border px-5 py-4">
                    <button
                        className="h-9 rounded-md border border-border bg-background px-3 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
                        disabled={checking || updating || restarting}
                        onClick={onCheck}
                        type="button"
                    >
                        {checking ? "Checking..." : "Refresh"}
                    </button>
                    <button
                        className="h-9 rounded-md border border-border bg-background px-3 text-xs font-medium hover:bg-muted disabled:opacity-50"
                        disabled={updating || restarting}
                        onClick={onClose}
                        type="button"
                    >
                        Cancel
                    </button>
                    <button
                        className="h-9 rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                        disabled={checking || updating || restarting || !info?.updateAvailable}
                        onClick={onConfirm}
                        type="button"
                    >
                        {restarting ? "Restarting..." : updating ? "Updating..." : "Update and restart"}
                    </button>
                </div>
            </div>
        </div>
    );
}

function VersionBlock({
    buildTime,
    label,
    version,
}: {
    buildTime?: string;
    label: string;
    version: string;
}) {
    return (
        <div className="rounded-md border border-border bg-background p-3">
            <p className="text-[11px] font-medium uppercase text-muted-foreground">
                {label}
            </p>
            <p className="mt-2 truncate text-sm font-semibold">{version}</p>
            <p className="mt-1 text-xs text-muted-foreground">
                {formatBuildTime(buildTime)}
            </p>
        </div>
    );
}

function displayVersion(version?: string, buildTime?: string) {
    return version || buildTime || "Unknown version";
}

function formatBuildTime(buildTime?: string) {
    if (!buildTime) return "Build time unknown";

    const date = new Date(buildTime);
    if (Number.isNaN(date.getTime())) {
        return buildTime;
    }

    return date.toLocaleString();
}
