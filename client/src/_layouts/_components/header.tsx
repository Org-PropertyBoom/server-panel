import { useState, useEffect } from "react";
import ColorModeSwitch from "_components/color-mode-switch";
import { User, Menu, RefreshCw } from "lucide-react";
import { useApp } from "../../_contexts/app";

type HeaderProps = {
    title: string;
    onMenuClick?: () => void;
};

export default function Header({ title, onMenuClick }: HeaderProps) {
    const { mode, isRoot } = useApp();
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const [checking, setChecking] = useState(false);
    const [updating, setUpdating] = useState(false);

    const checkUpdate = async () => {
        if (!isRoot) return;
        setChecking(true);
        try {
            const response = await fetch("/post/update");
            if (response.ok) {
                const data = await response.json();
                setUpdateAvailable(data.updateAvailable);
            }
        } catch (err) {
            console.error("Failed to check for updates", err);
        } finally {
            setChecking(false);
        }
    };

    useEffect(() => {
        if (isRoot) {
            checkUpdate();
            // Poll for updates every 3 minutes
            const interval = setInterval(checkUpdate, 180000);
            return () => clearInterval(interval);
        }
    }, [isRoot]);

    const handleUpdate = async () => {
        if (!updateAvailable) {
            await checkUpdate();
            return;
        }

        if (!window.confirm("A new update is available. Do you want to update and restart the server now?")) {
            return;
        }

        setUpdating(true);
        try {
            const response = await fetch("/post/update", { method: "POST" });
            if (response.ok) {
                window.alert("Update successful! The server is restarting. Please wait 3 seconds and reload the page.");
                setTimeout(() => {
                    window.location.reload();
                }, 3000);
            } else {
                const msg = await response.text();
                window.alert(`Update failed: ${msg}`);
            }
        } catch (err: any) {
            window.alert(`Update failed: ${err.message}`);
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
                        disabled={checking || updating}
                        className={`relative flex h-8 items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium transition-all ${
                            updateAvailable
                                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/20"
                                : "border-border bg-background hover:bg-muted text-muted-foreground hover:text-foreground"
                        }`}
                        title={updateAvailable ? "New update available!" : "Check for updates"}
                    >
                        <RefreshCw className={`h-3.5 w-3.5 ${checking || updating ? "animate-spin" : ""}`} />
                        <span>{updating ? "Updating..." : updateAvailable ? "Update Available" : "Check Update"}</span>
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
        </header>
    );
}
