import { useCallback, useState, useEffect, useRef } from "react";
import { Link } from "react-router-dom";
import ColorModeSwitch from "_components/color-mode-switch";
import { AlertTriangle, Boxes, Check, User, Menu, RefreshCw, X } from "lucide-react";
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
    const { mode, isRoot, headerApps } = useApp();
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
    const [checking, setChecking] = useState(false);
    const [updating, setUpdating] = useState(false);
    const [restarting, setRestarting] = useState(false);
    const [updateModalOpen, setUpdateModalOpen] = useState(false);
    const [updateError, setUpdateError] = useState("");
    const [updateSuccess, setUpdateSuccess] = useState(false);
    const [reloadCountdown, setReloadCountdown] = useState(0);
    const allowReload = useRef(false);
    const updateWorkflowActive = useRef(false);
    const [checkStatus, setCheckStatus] = useState<"idle" | "uptodate" | "error">("idle");
    const statusTimer = useRef<number | null>(null);

    const checkUpdate = useCallback(async () => {
        if (!isRoot) return null;
        setChecking(true);
        setUpdateError("");
        try {
            const response = await fetch("/post/update", { cache: "no-store" });
            if (!response.ok) {
                if (isRestartResponse(response.status)) return null;
                throw new Error(await responseError(response, "Failed to check for updates"));
            }
            const info: UpdateInfo = await response.json();

            setUpdateAvailable(info.updateAvailable);
            setUpdateInfo(info);
            return info;
        } catch (err: unknown) {
            const message =
                err instanceof Error ? err.message : "Failed to check for updates";
            if (!updateWorkflowActive.current) {
                setUpdateError(message);
            }
            console.error("Failed to check for updates:", err);
        } finally {
            setChecking(false);
        }

        return null;
    }, [isRoot]);

    useEffect(() => {
        if (isRoot) {
            checkUpdate();
            const interval = setInterval(checkUpdate, 5000);
            return () => clearInterval(interval);
        }
    }, [checkUpdate, isRoot]);

    useEffect(() => {
        if (!updating && !restarting) return;

        const handleBeforeUnload = (e: BeforeUnloadEvent) => {
            if (allowReload.current) return;
            e.preventDefault();
            e.returnValue =
                "The system is updating or restarting. Please do not close the browser or reload the page.";
            return e.returnValue;
        };

        window.addEventListener("beforeunload", handleBeforeUnload);
        return () =>
            window.removeEventListener("beforeunload", handleBeforeUnload);
    }, [updating, restarting]);

    const flashStatus = (status: "uptodate" | "error") => {
        setCheckStatus(status);
        if (statusTimer.current) window.clearTimeout(statusTimer.current);
        statusTimer.current = window.setTimeout(() => setCheckStatus("idle"), 3000);
    };

    useEffect(
        () => () => {
            if (statusTimer.current) window.clearTimeout(statusTimer.current);
        },
        [],
    );

    const handleUpdate = async () => {
        setUpdateError("");
        setUpdateSuccess(false);
        setReloadCountdown(0);

        if (updateAvailable) {
            setUpdateModalOpen(true);
            return;
        }

        // Manual "Check Update": re-check and give explicit feedback so the button
        // never feels dead when there is nothing to update.
        const info = await checkUpdate();
        if (info?.updateAvailable) {
            setUpdateModalOpen(true);
        } else if (info) {
            flashStatus("uptodate");
        } else {
            flashStatus("error");
        }
    };

    const confirmUpdate = async () => {
        updateWorkflowActive.current = true;
        setUpdating(true);
        setRestarting(false);
        setUpdateError("");
        setUpdateSuccess(false);
        try {
            const response = await fetch("/post/update", { method: "POST" });
            if (response.ok || isRestartResponse(response.status)) {
                const updateWasAccepted = response.ok;
                setRestarting(true);
                await waitForServer();
                setRestarting(false);
                const info = await checkUpdate();
                if (!updateWasAccepted && !info) {
                    throw new Error("The server reconnected, but the update status could not be confirmed.");
                }
                if (info?.updateAvailable) {
                    throw new Error("The server reconnected, but the update could not be confirmed.");
                }
                setUpdateSuccess(true);
                setUpdateAvailable(false);
                for (let seconds = 10; seconds > 0; seconds -= 1) {
                    setReloadCountdown(seconds);
                    await delay(1000);
                }
                setReloadCountdown(0);
                allowReload.current = true;
                window.location.reload();
            } else {
                setUpdateError(await responseError(response, "Update failed"));
            }
        } catch (err: any) {
            setUpdateError(err.message || "Update failed");
        } finally {
            updateWorkflowActive.current = false;
            setUpdating(false);
        }
    };

    const spinning = checking || updating || restarting;
    const updateLabel = restarting
        ? "Restarting..."
        : updating
          ? "Updating..."
          : updateAvailable
            ? "Update Available"
            : checkStatus === "uptodate"
              ? "Up to date"
              : checkStatus === "error"
                ? "Check failed"
                : "Check Update";
    const updateBtnClass = updateAvailable
        ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/20"
        : checkStatus === "uptodate"
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          : checkStatus === "error"
            ? "border-destructive/30 bg-destructive/10 text-destructive"
            : "border-border bg-background hover:bg-muted text-muted-foreground hover:text-foreground";
    const updateBtnTitle = updateAvailable
        ? "New update available!"
        : checkStatus === "error"
          ? updateError || "Update check failed"
          : checkStatus === "uptodate"
            ? "You're on the latest version"
            : "Check for updates";

    return (
        <header className="flex h-14 items-center justify-between border-b border-border bg-card pr-6">
            <div className="flex h-full min-w-0 items-center">
                <div className="flex h-full shrink-0 items-center gap-4 border-r border-border px-4 md:w-[280px] md:px-6">
                    {onMenuClick && (
                        <button
                            onClick={onMenuClick}
                            className="rounded-md p-1.5 hover:bg-muted md:hidden"
                            aria-label="Toggle menu"
                        >
                            <Menu className="h-5 w-5 text-muted-foreground" />
                        </button>
                    )}
                    <h1 className="truncate text-sm font-semibold text-foreground md:text-base">
                        {title}
                    </h1>
                </div>
                <div className="ml-4 flex min-w-0 items-center gap-2">
                    {headerApps.map((app) => (
                        <Link
                            key={app}
                            to={`/settings/apps/${encodeURIComponent(app)}`}
                            className="hidden h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium capitalize text-muted-foreground hover:bg-muted hover:text-foreground sm:flex"
                            title={`Open ${app}`}
                        >
                            <Boxes className="h-3.5 w-3.5" />
                            {app === "node" ? "Node.js" : app}
                        </Link>
                    ))}
                </div>
            </div>

            <div className="flex shrink-0 items-center gap-4">
                {isRoot && (
                    <button
                        onClick={handleUpdate}
                        disabled={checking || updating || restarting}
                        className={`relative flex h-8 items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium transition-all ${updateBtnClass}`}
                        title={updateBtnTitle}
                    >
                        {updateAvailable || updating || restarting ? (
                            <RefreshCw className={`h-3.5 w-3.5 ${spinning ? "animate-spin" : ""}`} />
                        ) : checkStatus === "uptodate" ? (
                            <Check className="h-3.5 w-3.5" />
                        ) : checkStatus === "error" ? (
                            <AlertTriangle className="h-3.5 w-3.5" />
                        ) : (
                            <RefreshCw className={`h-3.5 w-3.5 ${checking ? "animate-spin" : ""}`} />
                        )}
                        <span>{updateLabel}</span>
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
                    reloadCountdown={reloadCountdown}
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
    reloadCountdown,
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
    reloadCountdown: number;
    onCheck: () => Promise<UpdateInfo | null>;
    onClose: () => void;
    onConfirm: () => void;
}) {
    const localVersion = displayVersion(
        info?.localVersion,
        info?.localBuildTime,
    );
    const remoteVersion = displayVersion(
        info?.remoteVersion,
        info?.remoteBuildTime,
    );

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 backdrop-blur-sm">
            <div className="w-full max-w-lg rounded-md border border-border bg-card text-card-foreground shadow-lg">
                <div className="flex items-center justify-between border-b border-border px-5 py-4">
                    <div className="flex items-center gap-3">
                        <span className="flex h-9 w-9 items-center justify-center rounded-md border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                            <RefreshCw className="h-4 w-4" />
                        </span>
                        <div>
                            <h2 className="text-sm font-semibold">
                                Update server
                            </h2>
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
                        The server will download the new binary, replace the
                        current executable, and restart.
                    </div>

                    {(updating || restarting) && reloadCountdown === 0 && (
                        <div className="flex gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-xs text-amber-700 dark:text-amber-300 animate-pulse">
                            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
                            <div className="space-y-1">
                                <p className="font-semibold text-amber-800 dark:text-amber-200">
                                    {restarting
                                        ? "System is restarting..."
                                        : "System is updating..."}
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
                            Update installed. The server is online and the
                            connection has been restored.
                            {reloadCountdown > 0
                                ? ` Reloading in ${reloadCountdown} seconds...`
                                : null}
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
                        disabled={
                            checking ||
                            updating ||
                            restarting ||
                            !info?.updateAvailable
                        }
                        onClick={onConfirm}
                        type="button"
                    >
                        {reloadCountdown > 0
                            ? `Reloading in ${reloadCountdown}s`
                            : restarting
                              ? "Restarting..."
                              : updating
                                ? "Updating..."
                                : "Update and restart"}
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

async function waitForServer() {
    // Give the old process enough time to exit before checking the replacement.
    await delay(1500);

    let consecutiveSuccesses = 0;
    for (let attempt = 0; attempt < 90; attempt += 1) {
        try {
            const response = await fetch(`/healthz?reconnect=${Date.now()}`, {
                cache: "no-store",
            });
            consecutiveSuccesses = response.ok ? consecutiveSuccesses + 1 : 0;
            if (consecutiveSuccesses >= 2) return;
        } catch {
            consecutiveSuccesses = 0;
        }
        await delay(1000);
    }

    throw new Error("The update was installed, but the server did not reconnect in time.");
}

function delay(milliseconds: number) {
    return new Promise<void>((resolve) => window.setTimeout(resolve, milliseconds));
}

function isRestartResponse(status: number) {
    return status === 502 || status === 503 || status === 504;
}

async function responseError(response: Response, fallback: string) {
    const contentType = response.headers.get("content-type")?.toLowerCase() ?? "";
    if (!contentType.includes("text/plain") && !contentType.includes("application/json")) {
        return `${fallback} (${response.status})`;
    }

    const message = (await response.text()).trim();
    if (!message || /<\/?[a-z][\s\S]*>/i.test(message)) {
        return `${fallback} (${response.status})`;
    }

    return message.length > 240 ? `${message.slice(0, 237)}...` : message;
}
