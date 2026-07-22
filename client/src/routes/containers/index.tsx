import { useCallback, useEffect, useState } from "react";
import { Container as ContainerIcon, ExternalLink, FileCode2, FileText, Hammer, Info, Loader2, Play, Plus, RefreshCw, RotateCw, Save, Square, X } from "lucide-react";

import { toast } from "sonner";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";
import { runtime } from "runtime";

type ContainerRecord = {
    id: string;
    name: string;
    image: string;
    command?: string;
    engine: "docker" | "podman";
    owner: string;
    state: string;
    status: string;
    createdAt?: string;
    ports: string[];
    routeHosts?: string[];
    routeTenantCount?: number;
    routeTenantStack?: string;
};

type ContainerDetails = {
    id: string;
    name: string;
    image: string;
    imageId?: string;
    created?: string;
    platform?: string;
    engine: string;
    owner: string;
    command?: string;
    entrypoint?: string;
    workingDir?: string;
    user?: string;
    restartPolicy?: string;
    state: {
        status?: string;
        running: boolean;
        exitCode: number;
        startedAt?: string;
        finishedAt?: string;
        health?: string;
        restartCount?: number;
    };
    env?: string[];
    labels?: Record<string, string>;
    ports?: { container: string; host?: string }[];
    mounts?: { type?: string; source?: string; destination?: string; mode?: string; rw: boolean }[];
    networks?: { name: string; ipAddress?: string; gateway?: string; macAddress?: string }[];
    sizeRw?: number;
    sizeRootFs?: number;
    raw?: string;
};

// fmtSize renders a byte count as B/KB/MB/GB; undefined when size wasn't computed
// (e.g. rootless Podman, whose inspect has no --size).
function fmtSize(n?: number): string | undefined {
    if (n === undefined || n === null) return undefined;
    if (n < 1024) return `${n} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
}

// DetailRow is one label/value line in the details drawer; hidden when empty.
function DetailRow({ label, value, mono }: { label: string; value?: string | number | null; mono?: boolean }) {
    if (value === undefined || value === null || value === "") return null;
    return (
        <div className="flex gap-3 py-1.5">
            <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
            <span className={`min-w-0 flex-1 break-words text-foreground ${mono ? "font-mono text-[11px]" : ""}`}>{value}</span>
        </div>
    );
}

// formatTs renders an inspect timestamp in local time, hiding the empty/zero
// sentinel Docker uses for unset times ("0001-01-01T00:00:00Z").
function formatTs(value?: string): string | undefined {
    if (!value || value.startsWith("0001-01-01")) return undefined;
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return value;
    return d.toLocaleString();
}

function DetailSection({ title, count, children }: { title: string; count?: number; children: React.ReactNode }) {
    return (
        <section className="border-t border-border px-5 py-4">
            <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                {title}
                {count !== undefined ? <span className="ml-1.5 text-muted-foreground/60">({count})</span> : null}
            </h3>
            {children}
        </section>
    );
}

// RouteCell shows the reverse route view: which hostnames point at this container
// (App-route hostnames + a tenant-site count), the mirror of the /vhosts view.
function RouteCell({ container }: { container: ContainerRecord }) {
    const apps = container.routeHosts ?? [];
    const tenants = container.routeTenantCount ?? 0;
    if (apps.length === 0 && tenants === 0) {
        return <span className="text-muted-foreground">—</span>;
    }
    return (
        <div className="flex flex-wrap items-center gap-1.5">
            {apps.map((h) => (
                <a
                    key={h}
                    href={`https://${h}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-0.5 rounded border border-sky-500/30 bg-sky-500/10 px-1.5 py-0.5 font-mono text-[11px] text-sky-600 underline decoration-sky-600/30 underline-offset-2 hover:decoration-sky-600 dark:text-sky-400 dark:decoration-sky-400/30"
                    title={`Open https://${h} in a new tab`}
                >
                    {h}
                    <ExternalLink className="h-2.5 w-2.5 opacity-60" />
                </a>
            ))}
            {tenants > 0 ? (
                <span
                    className="inline-flex items-center gap-1 rounded-full border border-primary/20 bg-primary/10 px-2 py-0.5 text-[11px] font-semibold text-primary"
                    title={`${tenants} tenant site(s) via ${container.routeTenantStack ?? "this stack"}`}
                >
                    {tenants} tenant{container.routeTenantStack ? ` · ${container.routeTenantStack}` : ""}
                </span>
            ) : null}
        </div>
    );
}

export default function ContainersRoute() {
    const [containers, setContainers] = useState<ContainerRecord[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [actionLoading, setActionLoading] = useState("");
    const [logsContainer, setLogsContainer] = useState<ContainerRecord | null>(null);
    const [logs, setLogs] = useState("");
    const [logsLoading, setLogsLoading] = useState(false);
    const [logsError, setLogsError] = useState("");
    const [dockerfileContainer, setDockerfileContainer] = useState<ContainerRecord | null>(null);
    const [dockerfileContent, setDockerfileContent] = useState("");
    const [dockerfilePath, setDockerfilePath] = useState("");
    const [dockerfileLoading, setDockerfileLoading] = useState(false);
    const [dockerfileSaving, setDockerfileSaving] = useState(false);
    const [dockerfileError, setDockerfileError] = useState("");
    const [detailsContainer, setDetailsContainer] = useState<ContainerRecord | null>(null);
    const [details, setDetails] = useState<ContainerDetails | null>(null);
    const [detailsLoading, setDetailsLoading] = useState(false);
    const [detailsError, setDetailsError] = useState("");
    const [showRaw, setShowRaw] = useState(false);
    const [rebuilding, setRebuilding] = useState(false);
    const [rebuildLog, setRebuildLog] = useState("");
    const [createOpen, setCreateOpen] = useState(false);

    const loadContainers = useCallback(async () => {
        setLoading(true);
        setError("");
        try {
            const response = await fetch(Api.current.containers, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load containers");
            const data: { containers?: ContainerRecord[] } = await response.json();
            setContainers(data.containers ?? []);
        } catch (loadError) {
            setError(loadError instanceof Error ? loadError.message : "Failed to load containers");
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadContainers();
    }, [loadContainers]);

    const runAction = async (container: ContainerRecord, action: "start" | "stop" | "restart") => {
        const key = `${container.engine}:${container.owner}:${container.id}:${action}`;
        const label = container.name || container.id.slice(0, 12);
        const done = action === "stop" ? "stopped" : action === "restart" ? "restarted" : "started";
        setActionLoading(key);
        try {
            const response = await fetch(`${Api.current.containers}/action`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ action, engine: container.engine, id: container.id, owner: container.owner }),
            });
            if (!response.ok) throw new Error((await response.text()) || `Failed to ${action} container`);
            toast.success(`${label} ${done}`);
            await loadContainers();
        } catch (actionError) {
            toast.error(actionError instanceof Error ? actionError.message : `Failed to ${action} container`);
        } finally {
            setActionLoading("");
        }
    };

    const openLogs = async (container: ContainerRecord) => {
        setLogsContainer(container);
        setLogs("");
        setLogsError("");
        setLogsLoading(true);
        try {
            const query = new URLSearchParams({ engine: container.engine, id: container.id, owner: container.owner });
            const response = await fetch(`${Api.current.containers}/logs?${query.toString()}`, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load container logs");
            const data: { logs?: string } = await response.json();
            setLogs(data.logs ?? "");
        } catch (logsLoadError) {
            setLogsError(logsLoadError instanceof Error ? logsLoadError.message : "Failed to load container logs");
        } finally {
            setLogsLoading(false);
        }
    };

    const openDetails = async (container: ContainerRecord) => {
        setDetailsContainer(container);
        setDetails(null);
        setDetailsError("");
        setShowRaw(false);
        setDetailsLoading(true);
        try {
            const query = new URLSearchParams({ engine: container.engine, id: container.id, owner: container.owner });
            const response = await fetch(`${Api.current.containers}/inspect?${query.toString()}`, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load container details");
            setDetails((await response.json()) as ContainerDetails);
        } catch (detailsLoadError) {
            setDetailsError(detailsLoadError instanceof Error ? detailsLoadError.message : "Failed to load container details");
        } finally {
            setDetailsLoading(false);
        }
    };

    const dockerfileQuery = (container: ContainerRecord) => new URLSearchParams({
        engine: container.engine,
        id: container.id,
        owner: container.owner,
    });

    const openDockerfile = async (container: ContainerRecord) => {
        setDockerfileContainer(container);
        setDockerfileContent("");
        setDockerfilePath("");
        setDockerfileError("");
        setRebuildLog("");
        setDockerfileLoading(true);
        try {
            const response = await fetch(`${Api.current.containers}/dockerfile?${dockerfileQuery(container)}`, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Dockerfile not found");
            const data: { content?: string; path?: string } = await response.json();
            setDockerfileContent(data.content ?? "");
            setDockerfilePath(data.path ?? "");
        } catch (dockerfileLoadError) {
            setDockerfileError(dockerfileLoadError instanceof Error ? dockerfileLoadError.message : "Dockerfile not found");
        } finally {
            setDockerfileLoading(false);
        }
    };

    const putDockerfile = async (): Promise<boolean> => {
        if (!dockerfileContainer) return false;
        const response = await fetch(`${Api.current.containers}/dockerfile?${dockerfileQuery(dockerfileContainer)}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ content: dockerfileContent }),
        });
        if (!response.ok) throw new Error((await response.text()) || "Failed to save Dockerfile");
        return true;
    };

    const saveDockerfile = async () => {
        setDockerfileSaving(true);
        setDockerfileError("");
        try {
            await putDockerfile();
            toast.success("Dockerfile saved");
            setDockerfileContainer(null);
        } catch (dockerfileSaveError) {
            setDockerfileError(dockerfileSaveError instanceof Error ? dockerfileSaveError.message : "Failed to save Dockerfile");
        } finally {
            setDockerfileSaving(false);
        }
    };

    // saveAndRebuild writes the Dockerfile then runs `docker compose up -d --build
    // --no-deps <service>`. Compose only recreates the container on a successful
    // build, so a bad edit leaves the running one untouched. The build log streams
    // back into the modal.
    const saveAndRebuild = async () => {
        if (!dockerfileContainer) return;
        setDockerfileError("");
        setRebuildLog("");
        setDockerfileSaving(true);
        try {
            await putDockerfile();
        } catch (saveError) {
            setDockerfileError(saveError instanceof Error ? saveError.message : "Failed to save Dockerfile");
            setDockerfileSaving(false);
            return;
        }
        setDockerfileSaving(false);
        setRebuilding(true);
        try {
            const response = await fetch(`${Api.current.containers}/rebuild`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ engine: dockerfileContainer.engine, id: dockerfileContainer.id, owner: dockerfileContainer.owner }),
            });
            const data: { output?: string; error?: string } = await response.json();
            setRebuildLog(data.output || "");
            if (data.error) {
                toast.error(data.error);
            } else {
                toast.success(`${dockerfileContainer.name || "Container"} rebuilt`);
                await loadContainers();
            }
        } catch (rebuildError) {
            toast.error(rebuildError instanceof Error ? rebuildError.message : "Rebuild failed");
        } finally {
            setRebuilding(false);
        }
    };

    return (
        <DashboardLayout
            title="Containers"
            description="View Docker system containers and isolated rootless Podman containers."
            wide
            actions={
                <div className="flex items-center gap-2">
                    {runtime.isRoot ? (
                        <Button size="sm" className="gap-2" onClick={() => setCreateOpen(true)}>
                            <Plus className="h-4 w-4" />
                            New container
                        </Button>
                    ) : null}
                    <Button variant="outline" size="sm" className="gap-2" onClick={loadContainers} disabled={loading}>
                        <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                        Refresh
                    </Button>
                </div>
            }
        >
            {error ? (
                <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                    {error}
                </div>
            ) : null}

            {!error && !loading && containers.length === 0 ? (
                <div className="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-border text-center">
                    <ContainerIcon className="mb-3 h-9 w-9 text-muted-foreground/40" />
                    <p className="text-sm font-medium text-foreground">No containers found</p>
                    <p className="mt-1 text-xs text-muted-foreground">Docker and Podman are available from their respective user terminals.</p>
                </div>
            ) : null}

            {containers.length > 0 ? (
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[1240px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-3 font-medium">Container</th>
                                    <th className="px-4 py-3 font-medium">Engine</th>
                                    <th className="px-4 py-3 font-medium">Owner</th>
                                    <th className="px-4 py-3 font-medium">Image</th>
                                    <th className="px-4 py-3 font-medium">State</th>
                                    <th className="px-4 py-3 font-medium">Ports</th>
                                    <th className="px-4 py-3 font-medium">Routes</th>
                                    <th className="px-4 py-3 text-right font-medium">Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {containers.map((container) => (
                                    <tr key={`${container.engine}:${container.owner}:${container.id}`} className="hover:bg-muted/30">
                                        <td className="px-4 py-3">
                                            <p className="font-medium text-foreground">{container.name || container.id.slice(0, 12)}</p>
                                            <code className="mt-0.5 block text-[10px] text-muted-foreground">{container.id.slice(0, 12)}</code>
                                        </td>
                                        <td className="px-4 py-3">
                                            <span className="rounded border border-border bg-muted px-2 py-1 font-medium capitalize text-foreground">{container.engine}</span>
                                        </td>
                                        <td className="px-4 py-3 font-medium text-foreground">{container.owner}</td>
                                        <td className="max-w-64 truncate px-4 py-3 text-foreground" title={container.image}>{container.image || "—"}</td>
                                        <td className="px-4 py-3">
                                            <div className="flex items-center gap-2">
                                                <span className={`h-2 w-2 rounded-full ${container.state.toLowerCase() === "running" ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
                                                <span className="text-foreground">{container.status || container.state || "Unknown"}</span>
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 text-muted-foreground">
                                            {container.ports?.length ? container.ports.join(", ") : "—"}
                                        </td>
                                        <td className="px-4 py-3">
                                            <RouteCell container={container} />
                                        </td>
                                        <td className="px-4 py-3">
                                            <div className="flex items-center justify-end gap-1.5">
                                                <Button size="icon" variant="outline" className="h-8 w-8" title="Details" aria-label={`Inspect ${container.name}`} onClick={() => openDetails(container)}>
                                                    <Info className="h-3.5 w-3.5" />
                                                </Button>
                                                {container.state.toLowerCase() === "running" ? (
                                                    <Button size="icon" variant="outline" className="h-8 w-8" title="Stop" aria-label={`Stop ${container.name}`} disabled={Boolean(actionLoading)} onClick={() => runAction(container, "stop")}>
                                                        {actionLoading.endsWith(":stop") && actionLoading.includes(container.id) ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Square className="h-3.5 w-3.5" />}
                                                    </Button>
                                                ) : (
                                                    <Button size="icon" variant="outline" className="h-8 w-8 text-emerald-600" title="Start" aria-label={`Start ${container.name}`} disabled={Boolean(actionLoading)} onClick={() => runAction(container, "start")}>
                                                        {actionLoading.endsWith(":start") && actionLoading.includes(container.id) ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                                                    </Button>
                                                )}
                                                <Button size="icon" variant="outline" className="h-8 w-8" title="Restart" aria-label={`Restart ${container.name}`} disabled={Boolean(actionLoading) || container.state.toLowerCase() !== "running"} onClick={() => runAction(container, "restart")}>
                                                    {actionLoading.endsWith(":restart") && actionLoading.includes(container.id) ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RotateCw className="h-3.5 w-3.5" />}
                                                </Button>
                                                <Button size="icon" variant="outline" className="h-8 w-8" title="Logs" aria-label={`View logs for ${container.name}`} onClick={() => openLogs(container)}>
                                                    <FileText className="h-3.5 w-3.5" />
                                                </Button>
                                                <Button size="icon" variant="outline" className="h-8 w-8" title="Edit Dockerfile" aria-label={`Edit Dockerfile for ${container.name}`} onClick={() => openDockerfile(container)}>
                                                    <FileCode2 className="h-3.5 w-3.5" />
                                                </Button>
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            ) : null}

            {detailsContainer ? (
                <div className="fixed inset-0 z-50 flex justify-end bg-background/60 backdrop-blur-sm" onClick={() => setDetailsContainer(null)}>
                    <div
                        className="flex h-full w-full max-w-2xl flex-col overflow-hidden border-l border-border bg-card shadow-xl"
                        onClick={(event) => event.stopPropagation()}
                    >
                        <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
                            <div className="min-w-0">
                                <div className="flex items-center gap-2">
                                    <span className={`h-2 w-2 shrink-0 rounded-full ${details?.state.running ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
                                    <h2 className="truncate text-sm font-semibold text-foreground">{detailsContainer.name || detailsContainer.id.slice(0, 12)}</h2>
                                    {details?.state.health ? (
                                        <span className={`rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase ${details.state.health === "healthy" ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" : "bg-amber-500/10 text-amber-600 dark:text-amber-400"}`}>
                                            {details.state.health}
                                        </span>
                                    ) : null}
                                </div>
                                <code className="mt-1 block break-all text-[11px] text-muted-foreground">{detailsContainer.id}</code>
                            </div>
                            <Button size="icon" variant="ghost" className="h-8 w-8 shrink-0" onClick={() => setDetailsContainer(null)} aria-label="Close details">
                                <X className="h-4 w-4" />
                            </Button>
                        </div>
                        <div className="min-h-0 flex-1 overflow-auto text-xs">
                            {detailsLoading ? (
                                <div className="flex h-40 items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>
                            ) : detailsError ? (
                                <div className="m-5 rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-destructive">{detailsError.trim()}</div>
                            ) : details ? (
                                <>
                                    <DetailSection title="Overview">
                                        <DetailRow label="Engine" value={`${details.engine} · ${details.owner}`} />
                                        <DetailRow label="Image" value={details.image} mono />
                                        <DetailRow label="Image ID" value={details.imageId} mono />
                                        <DetailRow label="Platform" value={details.platform} />
                                        <DetailRow label="Created" value={formatTs(details.created)} />
                                        <DetailRow label="Size · writable" value={fmtSize(details.sizeRw)} />
                                        <DetailRow label="Size · total" value={details.sizeRootFs !== undefined ? `${fmtSize(details.sizeRootFs)} (incl. image)` : undefined} />
                                        <DetailRow label="Restart policy" value={details.restartPolicy} />
                                        <DetailRow label="Working dir" value={details.workingDir} mono />
                                        <DetailRow label="User" value={details.user} mono />
                                        <DetailRow label="Command" value={details.command} mono />
                                        <DetailRow label="Entrypoint" value={details.entrypoint} mono />
                                    </DetailSection>

                                    <DetailSection title="State">
                                        <DetailRow label="Status" value={details.state.status} />
                                        <DetailRow label="Running" value={details.state.running ? "yes" : "no"} />
                                        <DetailRow label="Health" value={details.state.health} />
                                        <DetailRow label="Exit code" value={details.state.running ? undefined : details.state.exitCode} />
                                        <DetailRow label="Restarts" value={details.state.restartCount} />
                                        <DetailRow label="Started" value={formatTs(details.state.startedAt)} />
                                        <DetailRow label="Finished" value={details.state.running ? undefined : formatTs(details.state.finishedAt)} />
                                    </DetailSection>

                                    {details.ports && details.ports.length > 0 ? (
                                        <DetailSection title="Ports" count={details.ports.length}>
                                            <div className="space-y-1 font-mono text-[11px]">
                                                {details.ports.map((p, i) => (
                                                    <div key={`${p.container}-${i}`} className="flex items-center gap-2 text-foreground">
                                                        <span>{p.container}</span>
                                                        {p.host ? <span className="text-muted-foreground">← {p.host}</span> : <span className="text-muted-foreground/60">not published</span>}
                                                    </div>
                                                ))}
                                            </div>
                                        </DetailSection>
                                    ) : null}

                                    {details.networks && details.networks.length > 0 ? (
                                        <DetailSection title="Networks" count={details.networks.length}>
                                            <div className="space-y-2">
                                                {details.networks.map((n) => (
                                                    <div key={n.name} className="rounded border border-border bg-muted/30 px-3 py-2">
                                                        <p className="font-medium text-foreground">{n.name}</p>
                                                        <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">
                                                            {n.ipAddress || "—"}{n.gateway ? ` · gw ${n.gateway}` : ""}{n.macAddress ? ` · ${n.macAddress}` : ""}
                                                        </p>
                                                    </div>
                                                ))}
                                            </div>
                                        </DetailSection>
                                    ) : null}

                                    {details.mounts && details.mounts.length > 0 ? (
                                        <DetailSection title="Mounts" count={details.mounts.length}>
                                            <div className="space-y-1.5 font-mono text-[11px]">
                                                {details.mounts.map((m, i) => (
                                                    <div key={`${m.destination}-${i}`} className="text-foreground">
                                                        <span className="break-all">{m.source || m.type}</span>
                                                        <span className="text-muted-foreground"> → {m.destination}</span>
                                                        <span className="text-muted-foreground/70"> {m.rw ? "rw" : "ro"}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        </DetailSection>
                                    ) : null}

                                    {details.env && details.env.length > 0 ? (
                                        <DetailSection title="Environment" count={details.env.length}>
                                            <p className="mb-2 text-[11px] text-amber-600 dark:text-amber-400">May contain secrets — visible to anyone with panel access.</p>
                                            <div className="space-y-0.5 font-mono text-[11px] text-foreground">
                                                {details.env.map((e, i) => {
                                                    const eq = e.indexOf("=");
                                                    const k = eq >= 0 ? e.slice(0, eq) : e;
                                                    const v = eq >= 0 ? e.slice(eq + 1) : "";
                                                    return (
                                                        <div key={`${k}-${i}`} className="break-all">
                                                            <span className="text-sky-600 dark:text-sky-400">{k}</span>
                                                            {eq >= 0 ? <span className="text-muted-foreground">={v}</span> : null}
                                                        </div>
                                                    );
                                                })}
                                            </div>
                                        </DetailSection>
                                    ) : null}

                                    {details.labels && Object.keys(details.labels).length > 0 ? (
                                        <DetailSection title="Labels" count={Object.keys(details.labels).length}>
                                            <div className="space-y-0.5 font-mono text-[11px] text-foreground">
                                                {Object.entries(details.labels).map(([k, v]) => (
                                                    <div key={k} className="break-all">
                                                        <span className="text-sky-600 dark:text-sky-400">{k}</span>
                                                        <span className="text-muted-foreground">: {v}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        </DetailSection>
                                    ) : null}

                                    {details.raw ? (
                                        <DetailSection title="Raw inspect">
                                            <Button variant="outline" size="sm" className="mb-2" onClick={() => setShowRaw((v) => !v)}>
                                                {showRaw ? "Hide" : "Show"} raw JSON
                                            </Button>
                                            {showRaw ? (
                                                <pre className="max-h-96 overflow-auto rounded-md border border-border bg-zinc-950 p-3 font-mono text-[11px] leading-5 text-zinc-200">{details.raw}</pre>
                                            ) : null}
                                        </DetailSection>
                                    ) : null}
                                </>
                            ) : null}
                        </div>
                    </div>
                </div>
            ) : null}

            {logsContainer ? (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
                    <div className="flex h-[min(720px,90vh)] w-full max-w-5xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl">
                        <div className="flex items-center justify-between gap-4 border-b border-border px-5 py-4">
                            <div className="min-w-0">
                                <h2 className="text-sm font-semibold text-foreground">{logsContainer.name || logsContainer.id} logs</h2>
                                <p className="mt-1 text-xs text-muted-foreground">Last 200 lines · {logsContainer.engine} · {logsContainer.owner}</p>
                            </div>
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setLogsContainer(null)} aria-label="Close logs">
                                <X className="h-4 w-4" />
                            </Button>
                        </div>
                        <div className="min-h-0 flex-1 overflow-auto bg-zinc-950 p-5">
                            {logsLoading ? (
                                <div className="flex h-full items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-zinc-400" /></div>
                            ) : logsError ? (
                                <p className="whitespace-pre-wrap text-xs text-red-400">{logsError.trim()}</p>
                            ) : (
                                <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-5 text-zinc-200">{logs || "No logs available."}</pre>
                            )}
                        </div>
                    </div>
                </div>
            ) : null}

            {dockerfileContainer ? (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
                    <div className="flex h-[min(760px,90vh)] w-full max-w-5xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl">
                        <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
                            <div className="min-w-0">
                                <h2 className="text-sm font-semibold text-foreground">Edit Dockerfile · {dockerfileContainer.name || dockerfileContainer.id}</h2>
                                <code className="mt-1 block break-all text-xs text-muted-foreground">{dockerfilePath || "Dockerfile path unavailable"}</code>
                            </div>
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setDockerfileContainer(null)} disabled={dockerfileSaving} aria-label="Close Dockerfile editor">
                                <X className="h-4 w-4" />
                            </Button>
                        </div>
                        {dockerfileError ? (
                            <div className="border-b border-destructive/20 bg-destructive/10 px-5 py-3 text-xs text-destructive">
                                {dockerfileError.trim()}
                                {!dockerfilePath ? <span className="mt-1 block text-muted-foreground">Add the label mthan.dockerfile=/absolute/path/Dockerfile when creating the container, or use Docker Compose from a directory containing Dockerfile.</span> : null}
                            </div>
                        ) : null}
                        <div className="min-h-0 flex-1 bg-background">
                            {dockerfileLoading ? (
                                <div className="flex h-full items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>
                            ) : (
                                <textarea value={dockerfileContent} onChange={(event) => setDockerfileContent(event.target.value)} disabled={!dockerfilePath} spellCheck={false} className="h-full w-full resize-none bg-transparent p-5 font-mono text-xs leading-6 text-foreground outline-none disabled:cursor-not-allowed disabled:opacity-50" aria-label="Dockerfile content" />
                            )}
                        </div>
                        {rebuildLog || rebuilding ? (
                            <div className="border-t border-border bg-zinc-950">
                                <div className="flex items-center gap-2 px-5 py-2 text-[11px] font-semibold uppercase tracking-wide text-zinc-400">
                                    {rebuilding ? <Loader2 className="h-3 w-3 animate-spin" /> : <Hammer className="h-3 w-3" />}
                                    {rebuilding ? "Rebuilding…" : "Build log"}
                                </div>
                                <pre className="max-h-56 overflow-auto px-5 pb-3 font-mono text-[11px] leading-5 text-zinc-200">{rebuildLog || "Running docker compose up --build…"}</pre>
                            </div>
                        ) : null}
                        <div className="flex items-center justify-between gap-3 border-t border-border px-5 py-3">
                            <p className="text-xs text-muted-foreground">
                                {dockerfileContainer.engine === "docker" ? "Rebuild runs compose up --build; the container is recreated only if the build succeeds." : "Saving does not rebuild the image or recreate the container."}
                            </p>
                            <div className="flex gap-2">
                                <Button variant="outline" size="sm" onClick={() => setDockerfileContainer(null)} disabled={dockerfileSaving || rebuilding}>Cancel</Button>
                                <Button variant="outline" size="sm" className="gap-2" onClick={saveDockerfile} disabled={!dockerfilePath || dockerfileLoading || dockerfileSaving || rebuilding}>
                                    {dockerfileSaving && !rebuilding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                                    Save
                                </Button>
                                {dockerfileContainer.engine === "docker" ? (
                                    <Button size="sm" className="gap-2" onClick={saveAndRebuild} disabled={!dockerfilePath || dockerfileLoading || dockerfileSaving || rebuilding}>
                                        {rebuilding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Hammer className="h-4 w-4" />}
                                        Save &amp; rebuild
                                    </Button>
                                ) : null}
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}

            {createOpen ? (
                <CreateContainerModal
                    onClose={() => setCreateOpen(false)}
                    onCreated={() => {
                        setCreateOpen(false);
                        loadContainers();
                    }}
                />
            ) : null}
        </DashboardLayout>
    );
}

// CreateContainerModal is the `docker run -d` form (root only). Multi-value fields
// (ports/env/volumes) are entered one-per-line and split before sending.
function CreateContainerModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
    const [image, setImage] = useState("");
    const [name, setName] = useState("");
    const [ports, setPorts] = useState("");
    const [env, setEnv] = useState("");
    const [volumes, setVolumes] = useState("");
    const [restart, setRestart] = useState("unless-stopped");
    const [creating, setCreating] = useState(false);
    const [output, setOutput] = useState("");

    const lines = (value: string) => value.split("\n").map((l) => l.trim()).filter(Boolean);

    const create = async () => {
        setCreating(true);
        setOutput("");
        try {
            const response = await fetch(`${Api.current.containers}/create`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    image: image.trim(),
                    name: name.trim(),
                    ports: lines(ports),
                    env: lines(env),
                    volumes: lines(volumes),
                    restart,
                }),
            });
            const data: { output?: string; error?: string } = await response.json();
            if (data.error) {
                setOutput(data.output || "");
                toast.error(data.error);
                return;
            }
            toast.success(`${name.trim() || "Container"} created`);
            onCreated();
        } catch (createError) {
            toast.error(createError instanceof Error ? createError.message : "Failed to create container");
        } finally {
            setCreating(false);
        }
    };

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => (creating ? null : onClose())}>
            <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl" onClick={(e) => e.stopPropagation()}>
                <div className="flex items-center justify-between border-b border-border px-5 py-4">
                    <div>
                        <h2 className="text-sm font-semibold text-foreground">New container</h2>
                        <p className="mt-0.5 text-xs text-muted-foreground">Runs <code>docker run -d</code> as root. For stack apps, use their deploy pipeline instead.</p>
                    </div>
                    <Button size="icon" variant="ghost" className="h-8 w-8" onClick={onClose} disabled={creating} aria-label="Close">
                        <X className="h-4 w-4" />
                    </Button>
                </div>
                <div className="min-h-0 flex-1 space-y-4 overflow-auto px-5 py-4 text-xs">
                    <label className="block">
                        <span className="mb-1 block font-medium text-foreground">Image <span className="text-destructive">*</span></span>
                        <input value={image} onChange={(e) => setImage(e.target.value)} placeholder="nocodb/nocodb:latest" autoFocus className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary" />
                    </label>
                    <label className="block">
                        <span className="mb-1 block font-medium text-foreground">Name</span>
                        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="optional — auto-generated if empty" className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary" />
                    </label>
                    <div className="grid grid-cols-2 gap-4">
                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">Ports</span>
                            <textarea value={ports} onChange={(e) => setPorts(e.target.value)} rows={3} placeholder={"one per line\n9001:8080"} className="w-full resize-none rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary" />
                            <span className="mt-1 block text-[11px] text-muted-foreground">host:container</span>
                        </label>
                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">Volumes</span>
                            <textarea value={volumes} onChange={(e) => setVolumes(e.target.value)} rows={3} placeholder={"one per line\n/data/nocodb:/usr/app/data"} className="w-full resize-none rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary" />
                            <span className="mt-1 block text-[11px] text-muted-foreground">src:dst[:ro]</span>
                        </label>
                    </div>
                    <label className="block">
                        <span className="mb-1 block font-medium text-foreground">Environment</span>
                        <textarea value={env} onChange={(e) => setEnv(e.target.value)} rows={3} placeholder={"one per line\nKEY=VALUE"} className="w-full resize-none rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary" />
                    </label>
                    <label className="block">
                        <span className="mb-1 block font-medium text-foreground">Restart policy</span>
                        <select value={restart} onChange={(e) => setRestart(e.target.value)} className="w-full rounded-md border border-border bg-background px-3 py-2 outline-none focus:border-primary">
                            <option value="unless-stopped">unless-stopped</option>
                            <option value="always">always</option>
                            <option value="on-failure">on-failure</option>
                            <option value="no">no</option>
                        </select>
                    </label>
                    {output ? <pre className="max-h-40 overflow-auto rounded-md border border-destructive/30 bg-zinc-950 p-3 font-mono text-[11px] leading-5 text-red-300">{output}</pre> : null}
                </div>
                <div className="flex items-center justify-end gap-2 border-t border-border px-5 py-3">
                    <Button variant="outline" size="sm" onClick={onClose} disabled={creating}>Cancel</Button>
                    <Button size="sm" className="gap-2" onClick={create} disabled={creating || !image.trim()}>
                        {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                        Create
                    </Button>
                </div>
            </div>
        </div>
    );
}
