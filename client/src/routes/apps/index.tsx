import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import {
    Boxes,
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
    Pencil,
    Save,
    X,
} from "lucide-react";

import { toast } from "sonner";

import { useApp } from "_contexts/app";
import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";
import SettingsSidebar from "../settings/sidebar";

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

type ConfigEditorTarget = {
    app: string;
    label: string;
    path: string;
};

export default function AppsRoute() {
    const { headerApps, isRoot, setHeaderApps } = useApp();
    const { app: requestedApp } = useParams<{ app?: string }>();
    const [apps, setApps] = useState<ServerApp[]>([
        {
            id: "0",
            name: "caddy",
            displayName: "Caddy",
            serviceName: "caddy.service",
            version: "System",
            port: "80, 443",
            description: "Primary public web server and automatic HTTPS reverse proxy.",
            running: true,
            uptime: "—",
            installed: false,
            manageable: true,
        },
        {
            id: "1",
            name: "nginx",
            displayName: "Nginx",
            serviceName: "nginx.service",
            version: "1.24.0",
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
    const [statusError, setStatusError] = useState("");
    const [configEditor, setConfigEditor] = useState<ConfigEditorTarget | null>(null);

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

    const installApp = async () => {
        if (!selectedApp) return;
        setActionLoading("install");
        try {
            const response = await fetch(Api.current.apps, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: selectedApp.name }) });
            if (!response.ok) throw new Error((await response.text()) || "Failed to install app");
            const data = await response.json();
            setApps((current) => current.map((app) => ({ ...app, ...(data.apps.find((status: ServerApp) => status.name === app.name) ?? {}) })));
            const status = data.apps.find((app: ServerApp) => app.name === selectedApp.name);
            if (status) setSelectedApp((current) => current ? { ...current, ...status } : current);
            toast.success(`${selectedApp.displayName} installed`);
        } catch (error) {
            toast.error(error instanceof Error ? error.message : "Failed to install app");
        } finally { setActionLoading(null); }
    };

    return (
        <DashboardLayout
            title="Settings"
            fullWidth={true}
        >
            <div className="grid h-full grid-cols-1 overflow-hidden md:grid-cols-[280px_1fr]">
                <SettingsSidebar section="apps" app={selectedApp?.name} />
                <main className="bg-background flex flex-col h-full overflow-hidden">
                    {selectedApp ? (
                        <div className="flex-1 space-y-4 overflow-y-auto p-4">
                            {statusError ? (
                                <p className="rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive">{statusError}</p>
                            ) : null}
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
                                <ContainerEngineConfiguration
                                    app={selectedApp}
                                    canEdit={isRoot}
                                    onEdit={setConfigEditor}
                                />
                            ) : null}

                            {selectedApp.name === "caddy" ? (
                                <section className="space-y-3">
                                    <div>
                                        <h3 className="text-sm font-semibold text-foreground">Caddy configuration</h3>
                                        <p className="mt-1 text-xs text-muted-foreground">Caddy is the primary public server for ports 80 and 443.</p>
                                    </div>
                                    <div className="relative rounded-md border border-border bg-card p-4">
                                        {isRoot ? (
                                            <Button size="sm" variant="outline" className="absolute right-3 top-3 h-7 gap-1.5 px-2 text-xs" onClick={() => setConfigEditor({ app: "caddy", label: "Caddyfile", path: "/etc/caddy/Caddyfile" })}>
                                                <Pencil className="h-3 w-3" />
                                                Edit
                                            </Button>
                                        ) : null}
                                        <div className="flex items-start gap-3 pr-16">
                                            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"><FileCode2 className="h-4 w-4" /></span>
                                            <div className="min-w-0">
                                                <p className="text-xs font-semibold text-foreground">Caddyfile</p>
                                                <code className="mt-1 block break-all text-xs text-primary">/etc/caddy/Caddyfile</code>
                                                <p className="mt-2 text-xs leading-5 text-muted-foreground">Global public host, TLS, reverse proxy, and route configuration.</p>
                                            </div>
                                        </div>
                                    </div>
                                </section>
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
            {configEditor ? (
                <AppConfigEditorModal target={configEditor} onClose={() => setConfigEditor(null)} />
            ) : null}
        </DashboardLayout>
    );
}

type ContainerEngineName = "docker" | "podman";

type ConfigurationItem = {
    label: string;
    value: string;
    icon: typeof Settings2;
    description: string;
    editablePath?: string;
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
                editablePath: "/etc/docker/daemon.json",
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
        summary: "These system-wide defaults apply to Podman installations. Individual rootless users can override them in ~/.config/containers/.",
        items: [
            {
                label: "Container configuration",
                value: "/etc/containers/containers.conf",
                icon: FileCode2,
                description: "Global runtime, networking, capabilities, and container behavior defaults.",
                editablePath: "/etc/containers/containers.conf",
            },
            {
                label: "Registry configuration",
                value: "/etc/containers/registries.conf",
                icon: Network,
                description: "Global registry search order, aliases, mirrors, and registry security settings.",
                editablePath: "/etc/containers/registries.conf",
            },
            {
                label: "Storage configuration",
                value: "/etc/containers/storage.conf",
                icon: HardDrive,
                description: "Global storage-driver defaults inherited by rootless users unless overridden.",
                editablePath: "/etc/containers/storage.conf",
            },
            {
                label: "Container policy",
                value: "/etc/containers/policy.json",
                icon: Settings2,
                description: "Global image-signature verification and trust policy.",
                editablePath: "/etc/containers/policy.json",
            },
            {
                label: "Rootless API socket",
                value: "$XDG_RUNTIME_DIR/podman/podman.sock",
                icon: Network,
                description: "Per-user Docker-compatible socket. It remains isolated and is not editable as a file.",
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

function ContainerEngineConfiguration({
    app,
    canEdit,
    onEdit,
}: {
    app: ServerApp;
    canEdit: boolean;
    onEdit: (target: ConfigEditorTarget) => void;
}) {
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
                        <div key={item.label} className="relative rounded-md border border-border bg-card p-4">
                            {canEdit && item.editablePath ? (
                                <Button
                                    size="sm"
                                    variant="outline"
                                    className="absolute right-3 top-3 h-7 gap-1.5 px-2 text-xs"
                                    onClick={() => onEdit({ app: engine, label: item.label, path: item.editablePath! })}
                                >
                                    <Pencil className="h-3 w-3" />
                                    Edit
                                </Button>
                            ) : null}
                            <div className="flex items-start gap-3">
                                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                                    <Icon className="h-4 w-4" />
                                </span>
                                <div className={`min-w-0 ${canEdit && item.editablePath ? "pr-16" : ""}`}>
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

function AppConfigEditorModal({ target, onClose }: { target: ConfigEditorTarget; onClose: () => void }) {
    const [content, setContent] = useState("");
    const [exists, setExists] = useState(true);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState("");

    useEffect(() => {
        let active = true;
        const load = async () => {
            try {
                const query = new URLSearchParams({ app: target.app, path: target.path });
                const response = await fetch(`/post/apps/config?${query.toString()}`, { cache: "no-store" });
                if (!response.ok) throw new Error((await response.text()) || "Failed to load configuration");
                const data: { content: string; exists: boolean } = await response.json();
                if (active) {
                    setContent(data.content ?? "");
                    setExists(data.exists);
                }
            } catch (loadError) {
                if (active) setError(loadError instanceof Error ? loadError.message : "Failed to load configuration");
            } finally {
                if (active) setLoading(false);
            }
        };
        load();
        return () => { active = false; };
    }, [target]);

    useEffect(() => {
        const closeOnEscape = (event: KeyboardEvent) => {
            if (event.key === "Escape" && !saving) onClose();
        };
        window.addEventListener("keydown", closeOnEscape);
        return () => window.removeEventListener("keydown", closeOnEscape);
    }, [onClose, saving]);

    const save = async () => {
        setSaving(true);
        setError("");
        try {
            const query = new URLSearchParams({ app: target.app, path: target.path });
            const response = await fetch(`/post/apps/config?${query.toString()}`, {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ content }),
            });
            if (!response.ok) throw new Error((await response.text()) || "Failed to save configuration");
            toast.success(`${target.label} saved`);
            setExists(true);
            onClose();
        } catch (saveError) {
            setError(saveError instanceof Error ? saveError.message : "Failed to save configuration");
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
            <div className="flex h-[min(720px,90vh)] w-full max-w-4xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl">
                <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
                    <div className="min-w-0">
                        <h2 className="text-sm font-semibold text-foreground">Edit {target.label}</h2>
                        <code className="mt-1 block break-all text-xs text-muted-foreground">{target.path}</code>
                    </div>
                    <Button size="icon" variant="ghost" className="h-8 w-8 shrink-0" onClick={onClose} disabled={saving} aria-label="Close editor">
                        <X className="h-4 w-4" />
                    </Button>
                </div>

                {!exists && !loading ? (
                    <div className="border-b border-amber-500/20 bg-amber-500/10 px-5 py-2 text-xs text-amber-700 dark:text-amber-300">
                        This file does not exist yet. Saving will create it.
                    </div>
                ) : null}
                {error ? (
                    <div className="border-b border-destructive/20 bg-destructive/10 px-5 py-2 text-xs text-destructive">{error.trim()}</div>
                ) : null}

                <div className="min-h-0 flex-1 bg-background">
                    {loading ? (
                        <div className="flex h-full items-center justify-center">
                            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                    ) : (
                        <textarea
                            value={content}
                            onChange={(event) => setContent(event.target.value)}
                            spellCheck={false}
                            className="h-full w-full resize-none bg-transparent p-5 font-mono text-xs leading-6 text-foreground outline-none"
                            aria-label={`${target.label} content`}
                        />
                    )}
                </div>

                <div className="flex items-center justify-between gap-3 border-t border-border px-5 py-3">
                    <p className="text-xs text-muted-foreground">Saving does not restart the container engine.</p>
                    <div className="flex gap-2">
                        <Button variant="outline" size="sm" onClick={onClose} disabled={saving}>Cancel</Button>
                        <Button size="sm" className="gap-2" onClick={save} disabled={loading || saving || Boolean(error && !content)}>
                            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                            Save
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}
