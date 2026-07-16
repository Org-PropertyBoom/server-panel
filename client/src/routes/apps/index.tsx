import { useEffect, useState } from "react";
import {
    Boxes,
    Cpu,
    RefreshCw,
    Play,
    Square,
    CheckCircle2,
    XCircle,
    Info,
    Terminal,
    Loader2,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";

interface ServerApp {
    id: string;
    name: string;
    displayName: string;
    serviceName: string;
    version: string;
    port: string | number;
    description: string;
    running: boolean;
    uptime: string;
    installed: boolean;
}

export default function AppsRoute() {
    const [apps, setApps] = useState<ServerApp[]>([
        {
            id: "1",
            name: "nginx",
            displayName: "Nginx Web Server",
            serviceName: "nginx.service",
            version: "1.24.0",
            port: "80, 443",
            description: "High-performance HTTP server, reverse proxy, and load balancer.",
            running: true,
            uptime: "2 days, 14 hours",
            installed: false,
        },
        {
            id: "2",
            name: "mariadb",
            displayName: "MariaDB Database",
            serviceName: "mariadb.service",
            version: "10.11.6",
            port: 3306,
            description: "Robust, open-source relational database management system.",
            running: true,
            uptime: "2 days, 14 hours",
            installed: false,
        },
        {
            id: "3",
            name: "php",
            displayName: "PHP-FPM Runtime",
            serviceName: "php8.2-fpm.service",
            version: "8.2.18",
            port: "Unix Socket",
            description: "PHP FastCGI Process Manager for processing dynamic web scripts.",
            running: true,
            uptime: "5 hours, 12 minutes",
            installed: false,
        },
        {
            id: "4",
            name: "redis",
            displayName: "Redis Cache",
            serviceName: "redis-server.service",
            version: "7.0.15",
            port: 6379,
            description: "In-memory data structure store used as a database, cache, and message broker.",
            running: false,
            uptime: "Stopped",
            installed: false,
        },
    ]);

    const [selectedApp, setSelectedApp] = useState<ServerApp | null>(apps[0]);
    const [actionLoading, setActionLoading] = useState<string | null>(null);
    const [statusLoading, setStatusLoading] = useState(true);
    const [statusError, setStatusError] = useState("");

    useEffect(() => {
        const loadStatuses = async () => {
            try {
                const response = await fetch(Api.current.apps, { cache: "no-store" });
                if (!response.ok) throw new Error("Failed to load app status");
                const data: { apps: Array<Pick<ServerApp, "name" | "installed" | "running">> } = await response.json();
                setApps((current) =>
                    current.map((app) => ({
                        ...app,
                        ...(data.apps.find((status) => status.name === app.name) ?? {}),
                    })),
                );
                setSelectedApp((current) => {
                    if (!current) return current;
                    const status = data.apps.find((item) => item.name === current.name);
                    return status ? { ...current, ...status } : current;
                });
                setStatusError("");
            } catch (error) {
                setStatusError(error instanceof Error ? error.message : "Failed to load app status");
            } finally {
                setStatusLoading(false);
            }
        };

        loadStatuses();
    }, []);

    const handleServiceAction = (id: string, action: "start" | "stop" | "restart") => {
        setActionLoading(action);
        setTimeout(() => {
            setApps((prev) =>
                prev.map((app) => {
                    if (app.id === id) {
                        const nextRunning = action === "restart" ? app.running : action === "start";
                        return {
                            ...app,
                            running: nextRunning,
                            uptime: nextRunning ? "Just started" : "Stopped",
                        };
                    }
                    return app;
                })
            );
            // Also update currently selected app details
            setSelectedApp((prev) => {
                if (prev && prev.id === id) {
                    const nextRunning = action === "restart" ? prev.running : action === "start";
                    return {
                        ...prev,
                        running: nextRunning,
                        uptime: nextRunning ? "Just started" : "Stopped",
                    };
                }
                return prev;
            });
            setActionLoading(null);
        }, 800);
    };

    return (
        <DashboardLayout
            title="System Apps"
            description="Monitor, configure, and control core system software services and runtimes."
            fullWidth={true}
        >
            <div className="grid h-full grid-cols-1 overflow-hidden md:grid-cols-[280px_1fr]">
                {/* Left Sidebar - Apps List */}
                <aside className="border-r border-border bg-card/60 flex flex-col h-full overflow-hidden select-none">
                    <div className="flex h-10 items-center justify-between px-3 border-b border-border bg-muted/20">
                        <span className="text-xs font-semibold text-muted-foreground">
                            System Apps
                        </span>
                        <Boxes className="h-4 w-4 text-muted-foreground" />
                    </div>

                    <div className="flex-1 overflow-y-auto py-2 px-2 space-y-1">
                        {statusError ? (
                            <p className="px-2 py-2 text-xs text-destructive">{statusError}</p>
                        ) : null}
                        {apps.map((app) => {
                            const isSelected = selectedApp?.id === app.id;
                            return (
                                <div
                                    key={app.id}
                                    className={`flex items-center gap-2.5 py-2 px-2.5 rounded-md cursor-pointer hover:bg-muted/60 transition-colors text-xs ${
                                        isSelected
                                            ? "bg-primary/10 text-primary font-semibold"
                                            : "text-foreground/90"
                                    }`}
                                    onClick={() => setSelectedApp(app)}
                                >
                                    <Cpu className={`h-4 w-4 shrink-0 ${isSelected ? "text-primary" : "text-muted-foreground"}`} />
                                    <span className="truncate flex-1 min-w-0">{app.displayName}</span>
                                    <span
                                        className={`h-2.5 w-2.5 shrink-0 rounded-full border ${
                                            statusLoading
                                                ? "animate-pulse border-muted-foreground/30 bg-muted"
                                                : app.installed
                                                  ? "border-emerald-500/30 bg-emerald-500"
                                                  : "border-muted-foreground/30 bg-transparent"
                                        }`}
                                        title={statusLoading ? "Checking installation" : app.installed ? "Installed" : "Not installed"}
                                    />
                                </div>
                            );
                        })}
                    </div>
                </aside>

                {/* Right Panel - App Detail View */}
                <main className="bg-background flex flex-col h-full overflow-hidden">
                    {selectedApp ? (
                        <div className="flex-1 overflow-y-auto p-6 space-y-6">
                            {/* Profile Header */}
                            <div className="flex items-start justify-between border-b border-border pb-5">
                                <div className="space-y-1.5">
                                    <div className="flex items-center gap-3">
                                        <h2 className="text-2xl font-bold tracking-tight text-foreground">
                                            {selectedApp.displayName}
                                        </h2>
                                        {!selectedApp.installed ? (
                                            <span className="inline-flex items-center gap-1 rounded border border-border bg-muted px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                                                <XCircle className="h-3 w-3 shrink-0" />
                                                Not installed
                                            </span>
                                        ) : selectedApp.running ? (
                                            <span className="inline-flex items-center gap-1 text-[10px] font-semibold text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20 uppercase tracking-wide">
                                                <CheckCircle2 className="h-3 w-3 shrink-0" />
                                                Running
                                            </span>
                                        ) : (
                                            <span className="inline-flex items-center gap-1 text-[10px] font-semibold text-muted-foreground bg-muted px-2 py-0.5 rounded border border-border uppercase tracking-wide">
                                                <XCircle className="h-3 w-3 shrink-0" />
                                                Stopped
                                            </span>
                                        )}
                                    </div>
                                    <p className="text-xs text-muted-foreground leading-relaxed max-w-2xl">
                                        {selectedApp.description}
                                    </p>
                                </div>
                            </div>

                            {/* Details Grid */}
                            <div className="grid grid-cols-1 md:grid-cols-3 border-b border-border bg-card/20">
                                <div className="p-5 border-b md:border-b-0 md:border-r border-border flex items-start gap-3">
                                    <Info className="h-5 w-5 text-primary shrink-0 mt-0.5" />
                                    <div className="space-y-1">
                                        <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider block">
                                            Software Version
                                        </span>
                                        <p className="font-semibold text-sm text-foreground">{selectedApp.version}</p>
                                    </div>
                                </div>

                                <div className="p-5 border-b md:border-b-0 md:border-r border-border flex items-start gap-3">
                                    <Terminal className="h-5 w-5 text-primary shrink-0 mt-0.5" />
                                    <div className="space-y-1 min-w-0">
                                        <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider block">
                                            Systemd Service
                                        </span>
                                        <p className="font-mono text-xs text-foreground truncate select-all" title={selectedApp.serviceName}>
                                            {selectedApp.serviceName}
                                        </p>
                                    </div>
                                </div>

                                <div className="p-5 flex items-start gap-3">
                                    <Cpu className="h-5 w-5 text-primary shrink-0 mt-0.5" />
                                    <div className="space-y-1 min-w-0">
                                        <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider block">
                                            Listen Port / Socket
                                        </span>
                                        <p className="font-mono text-xs text-foreground truncate select-all" title={String(selectedApp.port)}>
                                            {selectedApp.port}
                                        </p>
                                    </div>
                                </div>
                            </div>

                            {/* Service Control Panel */}
                            <div className="p-6 space-y-4">
                                <div className="flex items-center justify-between border-b border-border pb-2">
                                    <h3 className="text-sm font-semibold text-foreground">
                                        Service Management Console
                                    </h3>
                                    {selectedApp.running && (
                                        <span className="text-xs text-muted-foreground font-mono">
                                            Uptime: {selectedApp.uptime}
                                        </span>
                                    )}
                                </div>
                                <p className="text-xs text-muted-foreground leading-relaxed">
                                    Control status of {selectedApp.displayName} service block. Restart is recommended only during server configuration updates.
                                </p>
                                <div className="flex flex-wrap gap-2 pt-2">
                                    {!selectedApp.installed ? (
                                        <p className="text-xs text-muted-foreground">
                                            Install {selectedApp.displayName} to enable service controls.
                                        </p>
                                    ) : selectedApp.running ? (
                                        <>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                className="gap-2 text-amber-500 hover:text-amber-600 hover:bg-amber-500/10 border-amber-500/20"
                                                disabled={actionLoading !== null}
                                                onClick={() => handleServiceAction(selectedApp.id, "restart")}
                                            >
                                                <RefreshCw className={`h-4 w-4 ${actionLoading === "restart" ? "animate-spin" : ""}`} />
                                                Restart
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                className="gap-2 text-destructive hover:text-destructive hover:bg-destructive/10 border-destructive/20"
                                                disabled={actionLoading !== null}
                                                onClick={() => handleServiceAction(selectedApp.id, "stop")}
                                            >
                                                {actionLoading === "stop" ? (
                                                    <Loader2 className="h-4 w-4 animate-spin" />
                                                ) : (
                                                    <Square className="h-4 w-4 fill-current" />
                                                )}
                                                Stop Service
                                            </Button>
                                        </>
                                    ) : (
                                        <Button
                                            size="sm"
                                            className="gap-2 bg-emerald-600 hover:bg-emerald-700 text-white"
                                            disabled={actionLoading !== null}
                                            onClick={() => handleServiceAction(selectedApp.id, "start")}
                                        >
                                            {actionLoading === "start" ? (
                                                <Loader2 className="h-4 w-4 animate-spin" />
                                            ) : (
                                                <Play className="h-4 w-4 fill-current" />
                                            )}
                                            Start Service
                                        </Button>
                                    )}
                                </div>
                            </div>
                        </div>
                    ) : (
                        /* Empty state */
                        <div className="flex-grow flex flex-col items-center justify-center p-8 text-muted-foreground">
                            <Boxes className="h-12 w-12 text-muted-foreground/30 mb-2 shrink-0" />
                            <p className="text-sm font-medium">Select a system app from the list to manage it.</p>
                        </div>
                    )}
                </main>
            </div>
        </DashboardLayout>
    );
}
