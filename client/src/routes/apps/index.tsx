import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
    Boxes,
    Cpu,
    RefreshCw,
    Play,
    Square,
    CheckCircle2,
    XCircle,
    Loader2,
    Pin,
    PinOff,
    Download,
    FileCode2,
    HardDrive,
    Network,
    Settings2,
    TerminalSquare,
} from "lucide-react";

import { useApp } from "_contexts/app";
import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";

interface ServerApp {
    id: string;
    name: string;
    displayName: string;
    serviceName?: string;
    version: string;
    port?: string | number;
    description: string;
    running: boolean;
    uptime: string;
    installed: boolean;
    manageable: boolean;
    versions?: string[];
}

export default function AppsRoute() {
    const { headerApps, isRoot, setHeaderApps } = useApp();
    const navigate = useNavigate();
    const { app: requestedApp } = useParams<{ app?: string }>();
    const [apps, setApps] = useState<ServerApp[]>([
        {
            id: "1",
            name: "nginx",
            displayName: "Nginx",
            serviceName: "nginx.service",
            version: "1.24.0",
            port: "80, 443",
            description: "High-performance HTTP server, reverse proxy, and load balancer.",
            running: true,
            uptime: "2 days, 14 hours",
            installed: false,
            manageable: true,
        },
        {
            id: "2",
            name: "mariadb",
            displayName: "MariaDB",
            serviceName: "mariadb.service",
            version: "10.11.6",
            port: 3306,
            description: "Robust, open-source relational database management system.",
            running: true,
            uptime: "2 days, 14 hours",
            installed: false,
            manageable: true,
        },
        {
            id: "7",
            name: "node",
            displayName: "Node.js",
            version: "22",
            description: "System-wide Node.js 22 runtime.",
            running: false,
            uptime: "—",
            installed: false,
            manageable: false,
        },
        {
            id: "3",
            name: "php",
            displayName: "PHP",
            serviceName: "php-fpm.service",
            version: "Multiple",
            description: "PHP FastCGI Process Manager for processing dynamic web scripts.",
            running: true,
            uptime: "5 hours, 12 minutes",
            installed: false,
            manageable: true,
        },
        {
            id: "4",
            name: "redis",
            displayName: "Redis",
            serviceName: "redis-server.service",
            version: "7.0.15",
            port: 6379,
            description: "In-memory data structure store used as a database, cache, and message broker.",
            running: false,
            uptime: "Stopped",
            installed: false,
            manageable: true,
        },
        {
            id: "5",
            name: "docker",
            displayName: "Docker",
            serviceName: "docker.service",
            version: "System",
            description: "Container engine and runtime.",
            running: false,
            uptime: "Stopped",
            installed: false,
            manageable: true,
        },
        {
            id: "6",
            name: "podman",
            displayName: "Podman",
            version: "System",
            description: "Daemonless container engine.",
            running: false,
            uptime: "Stopped",
            installed: false,
            manageable: false,
        },
    ]);

    const [selectedApp, setSelectedApp] = useState<ServerApp | null>(apps[0]);
    const [actionLoading, setActionLoading] = useState<string | null>(null);
    const [statusLoading, setStatusLoading] = useState(true);
    const [statusError, setStatusError] = useState("");

    useEffect(() => {
        if (!requestedApp) {
            setSelectedApp(apps[0]);
            return;
        }
        const match = apps.find((app) => app.name === requestedApp);
        if (match) setSelectedApp(match);
    }, [requestedApp]);

    useEffect(() => {
        const loadStatuses = async () => {
            try {
                const response = await fetch(Api.current.apps, { cache: "no-store" });
                if (!response.ok) throw new Error("Failed to load app status");
                const data: {
                    apps: Array<Pick<ServerApp, "name" | "installed" | "manageable" | "running" | "serviceName" | "version" | "versions">>;
                } = await response.json();
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

    const selectApp = (app: ServerApp) => {
        setSelectedApp(app);
        navigate(`/apps/${encodeURIComponent(app.name)}`);
    };

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

    const installApp = async () => {
        if (!selectedApp) return;
        setActionLoading("install");
        setStatusError("");
        try {
            const response = await fetch(Api.current.apps, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: selectedApp.name }) });
            if (!response.ok) throw new Error((await response.text()) || "Failed to install app");
            const data = await response.json();
            setApps((current) => current.map((app) => ({ ...app, ...(data.apps.find((status: ServerApp) => status.name === app.name) ?? {}) })));
            const status = data.apps.find((app: ServerApp) => app.name === selectedApp.name);
            if (status) setSelectedApp((current) => current ? { ...current, ...status } : current);
        } catch (error) {
            setStatusError(error instanceof Error ? error.message : "Failed to install app");
        } finally { setActionLoading(null); }
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
                                    onClick={() => selectApp(app)}
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
                        <div className="flex-1 space-y-4 overflow-y-auto p-4">
                            {/* Compact app header */}
                            <div className="flex flex-wrap items-center gap-3 border-b border-border pb-4">
                                <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 gap-y-2">
                                    <div className="flex items-center gap-2">
                                        <h2 className="text-lg font-semibold tracking-tight text-foreground">
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
                                    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                                        <span>
                                            Version <strong className="font-medium text-foreground">{selectedApp.version}</strong>
                                        </span>
                                        {selectedApp.serviceName ? (
                                            <span>
                                                Service <code className="text-foreground">{selectedApp.serviceName}</code>
                                            </span>
                                        ) : null}
                                        {selectedApp.port !== undefined ? (
                                            <span>
                                                Port <code className="text-foreground">{selectedApp.port}</code>
                                            </span>
                                        ) : null}
                                    </div>
                                </div>

                                <div className="ml-auto flex shrink-0 items-center gap-2">
                                    {isRoot && !selectedApp.installed ? (
                                        <Button size="sm" className="gap-2" disabled={actionLoading !== null} onClick={installApp}>
                                            {actionLoading === "install" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
                                            Install
                                        </Button>
                                    ) : null}
                                    {selectedApp.installed && selectedApp.manageable ? (
                                        selectedApp.running ? (
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
                                        )
                                    ) : null}

                                    <Button
                                        size="icon"
                                        variant="outline"
                                        className="h-9 w-9"
                                        aria-label={headerApps.includes(selectedApp.name) ? "Remove from header" : "Add to header"}
                                        title={headerApps.includes(selectedApp.name) ? "Remove from header" : "Add to header"}
                                        onClick={() =>
                                            setHeaderApps(
                                                headerApps.includes(selectedApp.name)
                                                    ? headerApps.filter((app) => app !== selectedApp.name)
                                                    : [...headerApps, selectedApp.name],
                                            )
                                        }
                                    >
                                        {headerApps.includes(selectedApp.name) ? (
                                            <PinOff className="h-4 w-4" />
                                        ) : (
                                            <Pin className="h-4 w-4" />
                                        )}
                                    </Button>
                                </div>
                            </div>

                            {selectedApp.name === "php" ? (
                                <section className="space-y-3">
                                    <h3 className="text-sm font-semibold text-foreground">PHP configuration</h3>
                                    <div className="rounded-md border border-border bg-card p-4">
                                        <p className="text-xs font-medium text-muted-foreground">Installed versions</p>
                                        <div className="mt-3 flex flex-wrap gap-2">
                                            {selectedApp.versions?.length ? (
                                                selectedApp.versions.map((version) => (
                                                    <span
                                                        key={version}
                                                        className="rounded-md border border-border bg-muted px-2.5 py-1 font-mono text-xs text-foreground"
                                                    >
                                                        PHP {version}
                                                    </span>
                                                ))
                                            ) : (
                                                <span className="text-xs text-muted-foreground">No supported PHP version detected.</span>
                                            )}
                                        </div>
                                    </div>
                                </section>
                            ) : null}

                            {selectedApp.name === "docker" || selectedApp.name === "podman" ? (
                                <ContainerEngineConfiguration app={selectedApp} />
                            ) : null}
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

type ContainerEngineName = "docker" | "podman";

type ConfigurationItem = {
    label: string;
    value: string;
    icon: typeof Settings2;
    description: string;
};

const containerEngineDetails: Record<
    ContainerEngineName,
    {
        summary: string;
        items: ConfigurationItem[];
        commands: Array<{ label: string; command: string }>;
    }
> = {
    docker: {
        summary: "Docker uses a system daemon to manage containers, images, networks, and volumes.",
        items: [
            {
                label: "Daemon configuration",
                value: "/etc/docker/daemon.json",
                icon: FileCode2,
                description: "System-wide daemon options such as log drivers, registry mirrors, and address pools.",
            },
            {
                label: "Engine data",
                value: "/var/lib/docker",
                icon: HardDrive,
                description: "Default storage location for images, writable layers, volumes, and engine metadata.",
            },
            {
                label: "API socket",
                value: "/var/run/docker.sock",
                icon: Network,
                description: "Local Unix socket used by the Docker CLI and API clients.",
            },
            {
                label: "Client configuration",
                value: "~/.docker/config.json",
                icon: Settings2,
                description: "Per-user CLI preferences, credential helpers, and registry authentication settings.",
            },
        ],
        commands: [
            { label: "Engine information", command: "docker info" },
            { label: "Running containers", command: "docker ps" },
            { label: "All containers", command: "docker ps -a" },
            { label: "Disk usage", command: "docker system df" },
            { label: "Compose projects", command: "docker compose ls" },
        ],
    },
    podman: {
        summary: "Podman runs rootless inside each Linux user account. Containers, images, storage, and sockets are isolated from other panel users.",
        items: [
            {
                label: "User configuration",
                value: "~/.config/containers/containers.conf",
                icon: FileCode2,
                description: "Per-user runtime, networking, capabilities, and container behavior. Manage this from that user's account.",
            },
            {
                label: "User registries",
                value: "~/.config/containers/registries.conf",
                icon: Network,
                description: "Per-user registry search order, aliases, mirrors, and registry security settings.",
            },
            {
                label: "User storage configuration",
                value: "~/.config/containers/storage.conf",
                icon: HardDrive,
                description: "Storage driver and graph root overrides owned by the selected Linux user.",
            },
            {
                label: "User container data",
                value: "~/.local/share/containers/storage",
                icon: HardDrive,
                description: "Private images, writable layers, volumes, and metadata for this user only.",
            },
            {
                label: "User API socket",
                value: "$XDG_RUNTIME_DIR/podman/podman.sock",
                icon: Network,
                description: "Optional Docker-compatible socket scoped to the user's runtime directory; never shared globally.",
            },
            {
                label: "UID and GID mappings",
                value: "/etc/subuid and /etc/subgid",
                icon: Settings2,
                description: "Unique subordinate ID ranges provide each user with an isolated user namespace.",
            },
        ],
        commands: [
            { label: "Verify rootless mode", command: "podman info --format '{{.Host.Security.Rootless}}'" },
            { label: "Running containers", command: "podman ps" },
            { label: "All containers", command: "podman ps -a" },
            { label: "Disk usage", command: "podman system df" },
            { label: "Enable user API socket", command: "systemctl --user enable --now podman.socket" },
        ],
    },
};

function ContainerEngineConfiguration({ app }: { app: ServerApp }) {
    const engine = app.name as ContainerEngineName;
    const details = containerEngineDetails[engine];

    return (
        <section className="space-y-4">
            <div>
                <h3 className="text-sm font-semibold text-foreground">
                    {app.displayName} configuration
                </h3>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {details.summary}
                </p>
            </div>

            <div className="grid gap-3 lg:grid-cols-2">
                {details.items.map((item) => {
                    const Icon = item.icon;
                    return (
                        <div key={item.label} className="rounded-md border border-border bg-card p-4">
                            <div className="flex items-start gap-3">
                                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                                    <Icon className="h-4 w-4" />
                                </span>
                                <div className="min-w-0">
                                    <p className="text-xs font-semibold text-foreground">{item.label}</p>
                                    <code className="mt-1 block break-all text-xs text-primary">{item.value}</code>
                                    <p className="mt-2 text-xs leading-5 text-muted-foreground">{item.description}</p>
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>

            <div className="rounded-md border border-border bg-card">
                <div className="flex items-center gap-2 border-b border-border px-4 py-3">
                    <TerminalSquare className="h-4 w-4 text-muted-foreground" />
                    <h4 className="text-xs font-semibold text-foreground">Management commands</h4>
                </div>
                <div className="divide-y divide-border">
                    {details.commands.map((item) => (
                        <div key={item.command} className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                            <span className="text-xs text-muted-foreground">{item.label}</span>
                            <code className="break-all rounded bg-muted px-2 py-1 text-xs text-foreground">{item.command}</code>
                        </div>
                    ))}
                </div>
            </div>
        </section>
    );
}
