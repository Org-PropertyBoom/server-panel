import { Fragment, useCallback, useEffect, useState } from "react";
import { AlertTriangle, Ban, CheckCircle2, ChevronDown, ChevronRight, Container as ContainerIcon, ExternalLink, FileCode2, FileText, FolderGit2, GitCommit, Hammer, Info, Loader2, MoreVertical, Play, Plus, RefreshCw, Repeat2, RotateCw, Save, Square, Trash2, X, XCircle } from "lucide-react";

import { toast } from "sonner";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";
import { runtime } from "runtime";
import CreateContainerModal from "./create-container-modal";

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
    // Compose awareness. deployed=false means the compose file defines this
    // service but no container currently exists ("not deployed").
    project?: string;
    service?: string;
    workingDir?: string;
    deployed?: boolean;
    // managed = the panel holds this container's compose file. Unmanaged ones were
    // made by hand or by a stack pipeline — shown as-is, never retro-given a file.
    managed?: boolean;
    // "" means the image declares no HEALTHCHECK — unknown, not healthy.
    health?: string;
    // In-use guard: set on non-running rows whose project DIR is load-bearing
    // (a running container mounts out of it, or a host process runs from it).
    inUse?: boolean;
    inUseMounts?: { container: string; paths: string[] }[];
    inUseProcs?: string[];
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
        healthTest?: string;
        healthFailingStreak?: number;
        healthLastExit?: number;
        healthLastOutput?: string;
        restartCount?: number;
    };
    env?: string[];
    labels?: Record<string, string>;
    ports?: { container: string; host?: string }[];
    mounts?: { type?: string; source?: string; destination?: string; mode?: string; rw: boolean }[];
    networks?: { name: string; ipAddress?: string; gateway?: string; macAddress?: string }[];
    sizeRw?: number;
    sizeRootFs?: number;
    imageSize?: number;
    raw?: string;
};

type BuildStamp = {
    commit?: string;
    deployedAt?: string;
    ref?: string;
    repo?: string;
    source?: string;
    found: boolean;
};

// relTime renders an ISO timestamp as "2h ago"; undefined if empty/invalid.
function relTime(iso?: string): string | undefined {
    if (!iso) return undefined;
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return undefined;
    const s = Math.round((Date.now() - t) / 1000);
    if (s < 60) return `${s}s ago`;
    const m = Math.round(s / 60);
    if (m < 60) return `${m}m ago`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h}h ago`;
    return `${Math.round(h / 24)}d ago`;
}

// envVal pulls a KEY's value from a docker env array ("KEY=value").
function envVal(env: string[] | undefined, key: string): string | undefined {
    const hit = (env ?? []).find((e) => e.startsWith(key + "="));
    return hit ? hit.slice(key.length + 1) : undefined;
}

// githubCommitUrl derives a commit link from a repo ref ("Org/repo" or a github URL).
function githubCommitUrl(repo?: string, commit?: string): string | undefined {
    if (!repo || !commit) return undefined;
    let slug = repo.trim();
    const m = slug.match(/github\.com[/:]([^/]+\/[^/.]+)/i);
    if (m) slug = m[1];
    if (!/^[\w.-]+\/[\w.-]+$/.test(slug)) return undefined;
    return `https://github.com/${slug}/commit/${commit}`;
}

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

// SECRET_NEEDLES matches env var NAMES whose value is likely a credential. Mirrors
// go-actions' scrub list (app/services/container.go) so redaction is consistent
// across the platform. Case-insensitive substring match.
const SECRET_NEEDLES = ["PASS", "SECRET", "TOKEN", "KEY", "PWD", "CREDENTIAL", "DSN", "PRIVATE"];

function isSecretKey(key: string): boolean {
    const k = key.toUpperCase();
    return SECRET_NEEDLES.some((n) => k.includes(n));
}

// EnvRow renders one `KEY=VALUE` env line. Secret-looking values are masked by
// default (click to reveal) so credentials aren't exposed in plaintext to anyone
// with panel access.
function EnvRow({ entry }: { entry: string }) {
    const eq = entry.indexOf("=");
    const k = eq >= 0 ? entry.slice(0, eq) : entry;
    const v = eq >= 0 ? entry.slice(eq + 1) : "";
    const secret = eq >= 0 && v !== "" && isSecretKey(k);
    const [revealed, setRevealed] = useState(false);
    return (
        <div className="break-all">
            <span className="text-sky-600 dark:text-sky-400">{k}</span>
            {eq < 0 ? null : secret && !revealed ? (
                <>
                    <span className="text-muted-foreground">=</span>
                    <button
                        type="button"
                        onClick={() => setRevealed(true)}
                        className="ml-0.5 rounded bg-muted px-1.5 text-muted-foreground hover:text-foreground"
                        title="Click to reveal"
                    >
                        ••••••••
                    </button>
                </>
            ) : (
                <span className="text-muted-foreground">
                    ={v}
                    {secret ? (
                        <button type="button" onClick={() => setRevealed(false)} className="ml-1.5 text-[10px] uppercase tracking-wide text-muted-foreground/50 hover:text-foreground">
                            hide
                        </button>
                    ) : null}
                </span>
            )}
        </div>
    );
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

// fmtClock renders a wall-clock time; fmtDuration a short elapsed span.
function fmtClock(ms: number): string {
    return new Date(ms).toLocaleTimeString();
}
function fmtDuration(ms: number): string {
    const s = Math.max(0, Math.round(ms / 1000));
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
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

// inUseReasons expands the in-use detail into full, human sentences (for the
// hover title): which container mounts what, which host process runs from the dir.
function inUseReasons(c: ContainerRecord): string[] {
    const out: string[] = [];
    for (const m of c.inUseMounts ?? []) {
        const paths = m.paths.filter((p) => p !== ".");
        out.push(`${m.container} bind-mounts ${paths.length ? paths.join(", ") : "the directory"}`);
    }
    for (const p of c.inUseProcs ?? []) out.push(`host process: ${p}`);
    return out;
}

// inUseSummary is the compact inline reason ("go-actions: config, storage, …").
function inUseSummary(c: ContainerRecord): string {
    const parts: string[] = [];
    for (const m of c.inUseMounts ?? []) {
        const paths = m.paths.filter((p) => p !== ".");
        parts.push(`${m.container}: ${paths.length ? paths.join(", ") : "dir"}`);
    }
    const procs = c.inUseProcs ?? [];
    if (procs.length) parts.push(`host process${procs.length > 1 ? "es" : ""}`);
    return parts.join(" · ");
}

// InUseBadge warns that a not-deployed/stopped row's DIRECTORY is load-bearing —
// so it's not the dormant, safe-to-delete thing it looks like.
function InUseBadge({ container }: { container: ContainerRecord }) {
    if (!container.inUse) return null;
    const label = (container.inUseMounts ?? []).length ? "In use — live mounts" : "In use — host process";
    return (
        <span
            className="inline-flex items-center gap-1 rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-600 dark:text-amber-400"
            title={`This directory is load-bearing — deleting it would break live services:\n\n${inUseReasons(container).join("\n")}`}
        >
            <AlertTriangle className="h-3 w-3" />
            {label}
        </span>
    );
}

// projectHealth rolls a project's services up with AND: healthy only when EVERY
// service is. Services in a project are peers — there is no "the important one is
// fine". A green tick over a partly-dead project is the same class of lie as pc
// reporting openinary while actually serving from R2, which cost a day to spot.
//
// Returns "unhealthy" if any service is; "starting" while any is still coming up;
// "healthy" only if at least one reports healthy and none contradict it;
// "" when no service declares a HEALTHCHECK (unknown ≠ healthy).
function projectHealth(rows: ContainerRecord[]): string {
    const live = rows.filter((r) => r.deployed !== false);
    if (live.length === 0) return "";
    const states = live.map((r) => r.health ?? "");
    if (states.some((h) => h === "unhealthy")) return "unhealthy";
    if (states.some((h) => h === "starting")) return "starting";
    if (states.some((h) => h === "healthy")) return states.every((h) => h === "healthy") ? "healthy" : "partial";
    return "";
}

const HEALTH_TONE: Record<string, string> = {
    healthy: "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
    unhealthy: "border-destructive/30 bg-destructive/10 text-destructive",
    starting: "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400",
    partial: "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400",
};

function HealthChip({ health, title }: { health: string; title?: string }) {
    if (!health) return null;
    return (
        <span className={`inline-flex items-center rounded-full border px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide ${HEALTH_TONE[health] ?? ""}`} title={title}>
            {health === "partial" ? "not all healthy" : health}
        </span>
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
    const [rebuildStatus, setRebuildStatus] = useState<{ startedAt: number; finishedAt: number | null; ok: boolean | null } | null>(null);
    const [, setNowTick] = useState(0);

    // Tick once a second while a rebuild runs so the elapsed timer updates.
    useEffect(() => {
        if (!rebuilding) return;
        const t = window.setInterval(() => setNowTick((n) => n + 1), 1000);
        return () => window.clearInterval(t);
    }, [rebuilding]);
    const [createOpen, setCreateOpen] = useState(false);
    const [stamps, setStamps] = useState<Record<string, BuildStamp>>({});
    const [menuFor, setMenuFor] = useState("");
    const [removeContainer, setRemoveContainer] = useState<ContainerRecord | null>(null);
    const [removeText, setRemoveText] = useState("");
    const [showNotDeployed, setShowNotDeployed] = useState(true);
    const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
    const [projectBusy, setProjectBusy] = useState("");
    const [startTarget, setStartTarget] = useState<ContainerRecord | null>(null);

    // stampFor returns a container's resolved build stamp (first route host that has one).
    const stampFor = (c: ContainerRecord): BuildStamp | undefined =>
        (c.routeHosts ?? []).map((h) => stamps[h]).find((s) => s?.found);

    const loadContainers = useCallback(async () => {
        setLoading(true);
        setError("");
        try {
            const response = await fetch(Api.current.containers, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load containers");
            const data: { containers?: ContainerRecord[] } = await response.json();
            const list = data.containers ?? [];
            setContainers(list);
            // Resolve build stamps lazily + non-blocking so a slow backend never stalls the list.
            const hosts = Array.from(new Set(list.flatMap((c) => c.routeHosts ?? []).filter(Boolean)));
            if (hosts.length > 0) {
                fetch(`${Api.current.containers}/buildstamps`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ hosts }),
                })
                    .then((r) => (r.ok ? r.json() : null))
                    .then((d) => d && setStamps(d.stamps || {}))
                    .catch(() => undefined);
            }
        } catch (loadError) {
            setError(loadError instanceof Error ? loadError.message : "Failed to load containers");
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadContainers();
    }, [loadContainers]);

    const runAction = async (container: ContainerRecord, action: "start" | "stop" | "restart" | "kill" | "remove") => {
        const key = `${container.engine}:${container.owner}:${container.id}:${action}`;
        const label = container.name || container.id.slice(0, 12);
        const done = { stop: "stopped", restart: "restarted", kill: "killed", remove: "removed", start: "started" }[action];
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

    // Recreate: compose up --force-recreate (no rebuild) — re-applies compose config
    // / picks up a re-pulled image. Compose-managed containers only.
    const recreate = async (container: ContainerRecord) => {
        setActionLoading(`${container.id}:recreate`);
        try {
            const res = await fetch(`${Api.current.containers}/recreate`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ engine: container.engine, id: container.id, owner: container.owner }),
            });
            const data: { output?: string; error?: string } = await res.json();
            if (data.error) toast.error(data.error);
            else {
                toast.success(`${container.name || "Container"} recreated`);
                await loadContainers();
            }
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Recreate failed");
        } finally {
            setActionLoading("");
        }
    };

    // composeUp starts a not-deployed compose service (`docker compose up -d
    // <service>` in its project dir), bringing back a container that was removed
    // (e.g. by `docker compose down`).
    const composeUp = async (container: ContainerRecord) => {
        const key = `nd:${container.workingDir}:${container.service}`;
        setActionLoading(key);
        try {
            const res = await fetch(`${Api.current.containers}/compose-up`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ workingDir: container.workingDir, service: container.service }),
            });
            const data: { output?: string; error?: string } = await res.json();
            if (data.error) toast.error(data.error);
            else {
                toast.success(`${container.service || container.name} started`);
                await loadContainers();
            }
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Start failed");
        } finally {
            setActionLoading("");
        }
    };

    // Restart the whole project. Services in a compose project are peers, and
    // restarting one while leaving the rest on stale config is the less correct
    // default — so the project is the unit. Per-service actions stay on the rows.
    const restartProject = async (workingDir: string, label: string) => {
        setProjectBusy(workingDir);
        try {
            const res = await fetch(`${Api.current.containers}/compose-restart`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ workingDir }),
            });
            const data: { output?: string; error?: string } = await res.json();
            if (data.error) toast.error(data.error);
            else {
                toast.success(`${label} restarted`);
                await loadContainers();
            }
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Restart failed");
        } finally {
            setProjectBusy("");
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
        setRebuildStatus(null);
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
        const startedAt = Date.now();
        setRebuildStatus({ startedAt, finishedAt: null, ok: null });
        setRebuilding(true);
        try {
            const response = await fetch(`${Api.current.containers}/rebuild`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ engine: dockerfileContainer.engine, id: dockerfileContainer.id, owner: dockerfileContainer.owner }),
            });
            const data: { output?: string; error?: string } = await response.json();
            const log = data.output || "";
            setRebuildLog(data.error ? `${log}${log ? "\n\n" : ""}✗ ${data.error}` : log);
            setRebuildStatus({ startedAt, finishedAt: Date.now(), ok: !data.error });
            if (data.error) {
                toast.error(data.error);
            } else {
                toast.success(`${dockerfileContainer.name || "Container"} rebuilt`);
                await loadContainers();
            }
        } catch (rebuildError) {
            setRebuildStatus({ startedAt, finishedAt: Date.now(), ok: false });
            setRebuildLog((log) => log || String(rebuildError));
            toast.error(rebuildError instanceof Error ? rebuildError.message : "Rebuild failed");
        } finally {
            setRebuilding(false);
        }
    };

    // Compose-aware summary + grouping. A container with deployed===false is a
    // compose service with no running/stopped container ("not deployed").
    const isNotDeployed = (c: ContainerRecord) => c.deployed === false;
    const isRunning = (c: ContainerRecord) => c.deployed !== false && c.state.toLowerCase() === "running";
    const isStopped = (c: ContainerRecord) => c.deployed !== false && c.state.toLowerCase() !== "running";
    const counts = {
        running: containers.filter(isRunning).length,
        stopped: containers.filter(isStopped).length,
        notDeployed: containers.filter(isNotDeployed).length,
        standalone: containers.filter((c) => c.deployed !== false && !c.project).length,
    };

    // Group consecutive rows by compose project (the backend already sorts by
    // project, then name; standalone/non-compose sorts last). "Show not deployed"
    // hides the synthetic rows when the operator wants only live objects.
    const visibleContainers = showNotDeployed ? containers : containers.filter((c) => !isNotDeployed(c));
    type Group = { key: string; title: string; standalone: boolean; rows: ContainerRecord[] };
    const groups: Group[] = [];
    for (const c of visibleContainers) {
        const key = c.project || "__standalone__";
        const last = groups[groups.length - 1];
        if (!last || last.key !== key) {
            groups.push({ key, title: c.project || "Standalone", standalone: !c.project, rows: [c] });
        } else {
            last.rows.push(c);
        }
    }

    // Save, then start the build DETACHED. A long image build tied to the request
    // dies when the browser closes or the panel restarts on its own update — and
    // the panel does restart. setsid reparents it to init so it survives all of
    // that; the log path is where to follow it.
    const saveAndRebuildDetached = async () => {
        if (!dockerfileContainer) return;
        setDockerfileError("");
        setDockerfileSaving(true);
        try {
            await putDockerfile();
        } catch (saveError) {
            setDockerfileError(saveError instanceof Error ? saveError.message : "Failed to save Dockerfile");
            return;
        } finally {
            setDockerfileSaving(false);
        }
        try {
            const res = await fetch(`${Api.current.containers}/rebuild-detached`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ engine: dockerfileContainer.engine, id: dockerfileContainer.id, owner: dockerfileContainer.owner }),
            });
            const data: { logPath?: string; error?: string } = await res.json();
            if (data.error) {
                toast.error(data.error);
                return;
            }
            setRebuildLog(`Build started in the background. It keeps running if you close this panel or the panel restarts.\n\nFollow it with:\n  tail -f ${data.logPath}`);
            setRebuildStatus({ startedAt: Date.now(), finishedAt: Date.now(), ok: true });
            toast.success("Build started in the background");
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Could not start the build");
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
                <div className="mb-3 flex flex-wrap items-center justify-between gap-3 text-xs">
                    <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-muted-foreground">
                        <span className="inline-flex items-center gap-1.5">
                            <span className="h-2 w-2 rounded-full bg-emerald-500" />
                            <span className="font-medium text-foreground">{counts.running}</span> running
                        </span>
                        <span className="text-muted-foreground/40">·</span>
                        <span className="inline-flex items-center gap-1.5">
                            <span className="h-2 w-2 rounded-full bg-muted-foreground/40" />
                            <span className="font-medium text-foreground">{counts.stopped}</span> stopped
                        </span>
                        <span className="text-muted-foreground/40">·</span>
                        <span className="inline-flex items-center gap-1.5">
                            <span className="h-2 w-2 rounded-full border border-dashed border-muted-foreground/60" />
                            <span className="font-medium text-foreground">{counts.notDeployed}</span> not deployed
                        </span>
                        {counts.standalone > 0 ? (
                            <>
                                <span className="text-muted-foreground/40">·</span>
                                <span><span className="font-medium text-foreground">{counts.standalone}</span> standalone</span>
                            </>
                        ) : null}
                    </div>
                    {counts.notDeployed > 0 ? (
                        <label className="inline-flex cursor-pointer items-center gap-2 text-muted-foreground">
                            <input type="checkbox" checked={showNotDeployed} onChange={(e) => setShowNotDeployed(e.target.checked)} className="h-3.5 w-3.5 rounded border-border" />
                            Show not deployed
                        </label>
                    ) : null}
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
                                {groups.map((group) => {
                                    const run = group.rows.filter(isRunning).length;
                                    const nd = group.rows.filter(isNotDeployed).length;
                                    const noun = group.standalone ? "container" : "service";
                                    return (
                                        <Fragment key={group.key}>
                                            <tr className="bg-muted/30">
                                                <td colSpan={8} className="px-4 py-2">
                                                    <div className="flex flex-wrap items-center gap-2 text-[11px]">
                                                        {group.standalone ? (
                                                            <span className="font-semibold uppercase tracking-wide text-muted-foreground">Standalone</span>
                                                        ) : (
                                                            <>
                                                                <button
                                                                    onClick={() => setCollapsed((prev) => ({ ...prev, [group.key]: !prev[group.key] }))}
                                                                    className="inline-flex items-center gap-1.5 rounded px-1 py-0.5 font-semibold text-foreground hover:bg-muted"
                                                                    title={collapsed[group.key] ? "Show services" : "Hide services"}
                                                                >
                                                                    {collapsed[group.key] ? <ChevronRight className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
                                                                    <FolderGit2 className="h-3.5 w-3.5 text-muted-foreground" />
                                                                    {group.title}
                                                                </button>
                                                                {/* Health rolls up with AND — a project is healthy only when
                                                                    every service is. Never "the important one is fine". */}
                                                                <HealthChip
                                                                    health={projectHealth(group.rows)}
                                                                    title={group.rows.filter((r) => r.deployed !== false).map((r) => `${r.service || r.name}: ${r.health || "no healthcheck"}`).join("\n")}
                                                                />
                                                            </>
                                                        )}
                                                        <span className="text-muted-foreground">
                                                            {group.rows.length} {noun}{group.rows.length === 1 ? "" : "s"} · {run} running
                                                            {nd > 0 ? <span className="text-amber-600 dark:text-amber-400"> · {nd} not deployed</span> : null}
                                                        </span>
                                                        {!group.standalone && group.rows[0]?.workingDir ? (
                                                            <>
                                                                <span className="flex-1" />
                                                                <button
                                                                    onClick={() => restartProject(group.rows[0].workingDir as string, group.title)}
                                                                    disabled={Boolean(projectBusy)}
                                                                    className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
                                                                    title={`docker compose restart — the whole ${group.title} project, so no service is left on stale config`}
                                                                >
                                                                    {projectBusy === group.rows[0].workingDir ? <Loader2 className="h-3 w-3 animate-spin" /> : <RotateCw className="h-3 w-3" />}
                                                                    Restart project
                                                                </button>
                                                            </>
                                                        ) : null}
                                                    </div>
                                                </td>
                                            </tr>
                                            {(collapsed[group.key] ? [] : group.rows).map((container) =>
                                                isNotDeployed(container) ? (
                                                    <tr key={`nd:${container.workingDir}:${container.service}`} className={`hover:bg-muted/20 ${container.inUse ? "bg-amber-500/[0.04]" : "bg-muted/10"}`}>
                                                        <td className="px-4 py-3">
                                                            <p className="font-medium text-muted-foreground">{container.service || container.name}</p>
                                                            <code className="mt-0.5 block break-all text-[10px] text-muted-foreground/70">{container.workingDir}</code>
                                                            {container.inUse ? (
                                                                <p className="mt-1 flex items-start gap-1 text-[10px] text-amber-600 dark:text-amber-400" title={inUseReasons(container).join("\n")}>
                                                                    <AlertTriangle className="mt-px h-2.5 w-2.5 shrink-0" />
                                                                    <span className="break-words">In use — {inUseSummary(container)}. Directory is not safe to delete.</span>
                                                                </p>
                                                            ) : null}
                                                        </td>
                                                        <td className="px-4 py-3">
                                                            <span className="rounded border border-border/60 bg-muted/40 px-2 py-1 font-medium capitalize text-muted-foreground">docker</span>
                                                        </td>
                                                        <td className="px-4 py-3 text-muted-foreground">root</td>
                                                        <td className="px-4 py-3 text-muted-foreground/50">—</td>
                                                        <td className="px-4 py-3">
                                                            <div className="flex flex-wrap items-center gap-2">
                                                                <span className="inline-flex items-center gap-2">
                                                                    <span className="h-2 w-2 rounded-full border border-dashed border-muted-foreground/60" />
                                                                    <span className="rounded-full border border-border/60 bg-muted/40 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Not deployed</span>
                                                                </span>
                                                                <InUseBadge container={container} />
                                                            </div>
                                                        </td>
                                                        <td className="px-4 py-3 text-muted-foreground/50">—</td>
                                                        <td className="px-4 py-3 text-muted-foreground/50">—</td>
                                                        <td className="px-4 py-3">
                                                            <div className="flex items-center justify-end">
                                                                <Button
                                                                    size="sm"
                                                                    variant="outline"
                                                                    className="h-8 gap-1.5 text-emerald-600 dark:text-emerald-400"
                                                                    title={`Start ${container.service} (docker compose up -d)`}
                                                                    disabled={Boolean(actionLoading)}
                                                                    onClick={() => setStartTarget(container)}
                                                                >
                                                                    {actionLoading === `nd:${container.workingDir}:${container.service}` ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                                                                    Start
                                                                </Button>
                                                            </div>
                                                        </td>
                                                    </tr>
                                                ) : (
                                                    <tr key={`${container.engine}:${container.owner}:${container.id}`} className="hover:bg-muted/30">
                                                        <td className="px-4 py-3">
                                                            <p className="font-medium text-foreground">{container.name || container.id.slice(0, 12)}</p>
                                                            <code className="mt-0.5 block text-[10px] text-muted-foreground">{container.id.slice(0, 12)}</code>
                                                            {container.managed ? (
                                                                <span
                                                                    className="mt-0.5 inline-flex items-center rounded-full border border-primary/25 bg-primary/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-primary"
                                                                    title={`Managed by the panel — compose file at ${container.workingDir}/docker-compose.yml`}
                                                                >
                                                                    managed
                                                                </span>
                                                            ) : null}
                                                            {(() => {
                                                                const s = stampFor(container);
                                                                return s?.commit ? (
                                                                    <span
                                                                        className="mt-0.5 inline-flex items-center gap-1 text-[10px] text-emerald-600 dark:text-emerald-400"
                                                                        title={`commit ${s.commit}${s.deployedAt ? ` · deployed ${new Date(s.deployedAt).toLocaleString()}` : ""}`}
                                                                    >
                                                                        <GitCommit className="h-2.5 w-2.5" />
                                                                        {s.commit.slice(0, 7)}
                                                                        {s.deployedAt ? ` · ${relTime(s.deployedAt)}` : ""}
                                                                    </span>
                                                                ) : null;
                                                            })()}
                                                            {container.inUse ? (
                                                                <p className="mt-1 flex items-start gap-1 text-[10px] text-amber-600 dark:text-amber-400" title={inUseReasons(container).join("\n")}>
                                                                    <AlertTriangle className="mt-px h-2.5 w-2.5 shrink-0" />
                                                                    <span className="break-words">In use — {inUseSummary(container)}</span>
                                                                </p>
                                                            ) : null}
                                                        </td>
                                                        <td className="px-4 py-3">
                                                            <span className="rounded border border-border bg-muted px-2 py-1 font-medium capitalize text-foreground">{container.engine}</span>
                                                        </td>
                                                        <td className="px-4 py-3 font-medium text-foreground">{container.owner}</td>
                                                        <td className="max-w-64 truncate px-4 py-3 text-foreground" title={container.image}>{container.image || "—"}</td>
                                                        <td className="px-4 py-3">
                                                            <div className="flex flex-wrap items-center gap-2">
                                                                <span className="inline-flex items-center gap-2">
                                                                    <span className={`h-2 w-2 rounded-full ${container.state.toLowerCase() === "running" ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
                                                                    <span className="text-foreground">{container.status || container.state || "Unknown"}</span>
                                                                </span>
                                                                <HealthChip health={container.health ?? ""} />
                                                                <InUseBadge container={container} />
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
                                                                <div className="relative">
                                                                    <Button size="icon" variant="outline" className="h-8 w-8" title="More actions" aria-label={`More actions for ${container.name}`} onClick={() => setMenuFor(menuFor === container.id ? "" : container.id)}>
                                                                        {actionLoading === `${container.id}:recreate` ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <MoreVertical className="h-3.5 w-3.5" />}
                                                                    </Button>
                                                                    {menuFor === container.id ? (
                                                                        <>
                                                                            <div className="fixed inset-0 z-40" onClick={() => setMenuFor("")} />
                                                                            <div className="absolute right-0 z-50 mt-1 w-44 overflow-hidden rounded-md border border-border bg-card py-1 text-xs shadow-lg">
                                                                                <button onClick={() => { setMenuFor(""); recreate(container); }} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-foreground hover:bg-muted">
                                                                                    <Repeat2 className="h-3.5 w-3.5" /> Recreate
                                                                                </button>
                                                                                <button onClick={() => { setMenuFor(""); runAction(container, "kill"); }} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-foreground hover:bg-muted">
                                                                                    <Ban className="h-3.5 w-3.5" /> Force kill
                                                                                </button>
                                                                                <div className="my-1 border-t border-border" />
                                                                                <button onClick={() => { setMenuFor(""); setRemoveText(""); setRemoveContainer(container); }} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-destructive hover:bg-destructive/10">
                                                                                    <Trash2 className="h-3.5 w-3.5" /> Remove…
                                                                                </button>
                                                                            </div>
                                                                        </>
                                                                    ) : null}
                                                                </div>
                                                            </div>
                                                        </td>
                                                    </tr>
                                                )
                                            )}
                                        </Fragment>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                </div>
            ) : null}

            {detailsContainer ? (
                <div className="fixed inset-0 z-[60] flex justify-end bg-background/60 backdrop-blur-sm" onClick={() => setDetailsContainer(null)}>
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
                                        <DetailRow label="Image · pull size" value={details.imageSize !== undefined ? `${fmtSize(details.imageSize)} compressed` : undefined} />
                                        <DetailRow label="Writable layer" value={fmtSize(details.sizeRw)} />
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

                                    {details.state.health ? (
                                        <DetailSection title="Health check">
                                            <DetailRow label="Status" value={details.state.health} />
                                            <DetailRow label="Failing streak" value={details.state.healthFailingStreak} />
                                            <DetailRow label="Probe" value={details.state.healthTest} mono />
                                            {details.state.healthLastOutput || details.state.health !== "healthy" ? (
                                                <div className="mt-2">
                                                    <p className="text-muted-foreground">
                                                        Last check{details.state.healthLastExit !== undefined ? ` · exit ${details.state.healthLastExit}` : ""}
                                                    </p>
                                                    <pre className="mt-1 max-h-40 overflow-auto rounded border border-border bg-zinc-950 p-2 font-mono text-[11px] leading-5 text-zinc-200">{details.state.healthLastOutput || "(the probe produced no output)"}</pre>
                                                </div>
                                            ) : null}
                                        </DetailSection>
                                    ) : null}

                                    {(() => {
                                        const s = stampFor(detailsContainer);
                                        const commit = s?.commit || envVal(details.env, "BUILD_SHA");
                                        const deployedAt = s?.deployedAt || envVal(details.env, "DEPLOYED_AT");
                                        const hasRoute = (detailsContainer.routeHosts ?? []).length > 0;
                                        if (!commit && !hasRoute) return null;
                                        const gh = githubCommitUrl(s?.repo, commit);
                                        return (
                                            <DetailSection title="Build">
                                                {commit ? (
                                                    <>
                                                        <DetailRow label="Commit" value={commit.slice(0, 7)} mono />
                                                        <DetailRow label="Full commit" value={commit} mono />
                                                        <DetailRow
                                                            label="Deployed"
                                                            value={deployedAt ? `${new Date(deployedAt).toLocaleString()}${relTime(deployedAt) ? ` · ${relTime(deployedAt)}` : ""}` : undefined}
                                                        />
                                                        <DetailRow label="Ref" value={s?.ref} mono />
                                                        <DetailRow label="Source" value={s?.source === "header" ? "x-build-commit header" : s?.source === "up" ? "/up __BUILD__" : "container env"} />
                                                        {gh ? (
                                                            <div className="py-1.5">
                                                                <a href={gh} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-sky-600 hover:underline dark:text-sky-400">
                                                                    View commit on GitHub <ExternalLink className="h-3 w-3" />
                                                                </a>
                                                            </div>
                                                        ) : null}
                                                    </>
                                                ) : (
                                                    <p className="text-muted-foreground">
                                                        No build stamp on the route (no <code>x-build-commit</code> header or <code>/up</code> <code>__BUILD__</code>).
                                                    </p>
                                                )}
                                            </DetailSection>
                                        );
                                    })()}

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
                                            <p className="mb-2 text-[11px] text-muted-foreground">Secret-looking values are masked — click <span className="font-mono">••••••••</span> to reveal.</p>
                                            <div className="space-y-0.5 font-mono text-[11px] text-foreground">
                                                {details.env.map((e, i) => (
                                                    <EnvRow key={`${e.slice(0, e.indexOf("=") + 1)}-${i}`} entry={e} />
                                                ))}
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
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
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
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
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
                        {rebuildStatus ? (
                            <div className="border-t border-border bg-zinc-950">
                                <div className="flex flex-wrap items-center justify-between gap-2 px-5 py-2 text-[11px]">
                                    <span className="flex items-center gap-1.5 font-semibold uppercase tracking-wide">
                                        {rebuilding ? (
                                            <><Loader2 className="h-3.5 w-3.5 animate-spin text-amber-400" /><span className="text-amber-400">Rebuilding…</span></>
                                        ) : rebuildStatus.ok ? (
                                            <><CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" /><span className="text-emerald-400">Build succeeded</span></>
                                        ) : (
                                            <><XCircle className="h-3.5 w-3.5 text-red-400" /><span className="text-red-400">Build failed</span></>
                                        )}
                                    </span>
                                    <span className="font-mono text-zinc-500">
                                        {rebuilding
                                            ? `started ${fmtClock(rebuildStatus.startedAt)} · elapsed ${fmtDuration(Date.now() - rebuildStatus.startedAt)}`
                                            : `finished ${fmtClock(rebuildStatus.finishedAt ?? rebuildStatus.startedAt)} · took ${fmtDuration((rebuildStatus.finishedAt ?? rebuildStatus.startedAt) - rebuildStatus.startedAt)}`}
                                    </span>
                                </div>
                                <pre className="max-h-56 overflow-auto px-5 pb-3 font-mono text-[11px] leading-5 text-zinc-200">{rebuildLog || (rebuilding ? "Running docker compose up --build… (can take a few minutes; the full log appears when it finishes)" : "(no output)")}</pre>
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
                                    <>
                                        <Button variant="outline" size="sm" className="gap-2" onClick={saveAndRebuildDetached} disabled={!dockerfilePath || dockerfileLoading || dockerfileSaving || rebuilding}>
                                            <Hammer className="h-4 w-4" />
                                            Save &amp; build in background
                                        </Button>
                                        <Button size="sm" className="gap-2" onClick={saveAndRebuild} disabled={!dockerfilePath || dockerfileLoading || dockerfileSaving || rebuilding}>
                                            {rebuilding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Hammer className="h-4 w-4" />}
                                            Save &amp; rebuild
                                        </Button>
                                    </>
                                ) : null}
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}

            {removeContainer ? (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => setRemoveContainer(null)}>
                    <div className="w-full max-w-md rounded-md border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                        <h2 className="text-sm font-semibold text-foreground">Remove container</h2>
                        <p className="mt-2 text-xs text-muted-foreground">
                            This force-removes <b className="text-foreground">{removeContainer.name || removeContainer.id.slice(0, 12)}</b> (<code>docker rm -f</code>) — it stops and deletes the container. Its image and any named volumes are kept. A compose-managed service comes back on the next <code>up</code>.
                        </p>
                        <p className="mt-3 text-xs text-muted-foreground">
                            Type <b className="font-mono text-foreground">{removeContainer.name || removeContainer.id.slice(0, 12)}</b> to confirm:
                        </p>
                        <input
                            value={removeText}
                            onChange={(e) => setRemoveText(e.target.value)}
                            autoFocus
                            className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm outline-none focus:border-destructive"
                        />
                        <div className="mt-5 flex justify-end gap-2">
                            <Button variant="outline" size="sm" onClick={() => setRemoveContainer(null)}>Cancel</Button>
                            <Button
                                size="sm"
                                variant="destructive"
                                className="gap-2"
                                disabled={removeText.trim() !== (removeContainer.name || removeContainer.id.slice(0, 12))}
                                onClick={() => {
                                    const c = removeContainer;
                                    setRemoveContainer(null);
                                    runAction(c, "remove");
                                }}
                            >
                                <Trash2 className="h-4 w-4" /> Remove
                            </Button>
                        </div>
                    </div>
                </div>
            ) : null}

            {startTarget ? (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => setStartTarget(null)}>
                    <div className="w-full max-w-md rounded-md border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                        <h2 className="text-sm font-semibold text-foreground">Start service</h2>
                        <p className="mt-2 text-xs text-muted-foreground">
                            Deploy <b className="text-foreground">{startTarget.service}</b>
                            {startTarget.project ? <> from project <b className="text-foreground">{startTarget.project}</b></> : null}. This runs <code>docker compose up -d {startTarget.service}</code> in:
                        </p>
                        <code className="mt-2 block break-all rounded border border-border bg-muted/40 px-2 py-1.5 text-[11px] text-foreground">{startTarget.workingDir}</code>
                        <div className="mt-5 flex justify-end gap-2">
                            <Button variant="outline" size="sm" onClick={() => setStartTarget(null)}>Cancel</Button>
                            <Button
                                size="sm"
                                className="gap-2"
                                onClick={() => {
                                    const c = startTarget;
                                    setStartTarget(null);
                                    composeUp(c);
                                }}
                            >
                                <Play className="h-4 w-4" /> Start
                            </Button>
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
