import { useCallback, useEffect, useState, type ReactNode } from "react";
import {
    AlertTriangle,
    CheckCircle2,
    Database,
    Loader2,
    Lock,
    Pencil,
    Plus,
    RefreshCw,
    ShieldCheck,
    Trash2,
    Zap,
} from "lucide-react";
import { toast } from "sonner";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import { runtime } from "../../runtime";

type HostRow = {
    hostname: string;
    kind: string;
    stack?: string;
    upstream?: string;
    status: "in_sync" | "will_write" | "will_remove" | "orphan" | string;
};

type Skip = { table: string; host: string; reason: string };

type DryRun = {
    vhosts_dir: string;
    files: Array<{ name: string }>;
    hosts: HostRow[];
    would_write: string[];
    would_remove: string[];
    orphans: string[];
    skips?: Skip[];
    missing_tables?: string[];
    in_sync: boolean;
};

type ManageRow = {
    id: number;
    host: string;
    serverStack?: string;
    target: string;
    code?: number;
    isActive: boolean;
    softDeleted: boolean;
};

type ManageSets = {
    systemHosts: ManageRow[];
    redirects: ManageRow[];
    stacks: string[];
};

type VhostState = {
    configured: boolean;
    source?: string;
    vhostsDir: string;
    liveReload: boolean;
    message?: string;
    error?: string;
    dryRun?: DryRun;
    manage?: ManageSets;
};

type ReconcileResult = {
    reloaded: boolean;
    written: string[];
    removed: string[];
    removes_suppressed?: string[];
    orphans: string[];
    skips?: Skip[];
    adapt_warnings?: string[];
    missing_tables?: string[];
    backup_path?: string;
    error?: string;
    duration_ms: number;
};

type Source = { id: string; name: string; engine: string };

export default function VHostsRoute() {
    if (!runtime.isRoot) {
        return (
            <DashboardLayout title="Caddy VHosts" description="Manage the public hosts Caddy serves.">
                <div className="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-border text-center">
                    <Lock className="mb-3 h-8 w-8 text-muted-foreground/40" />
                    <p className="text-sm font-medium">VHost management is available in the root session.</p>
                </div>
            </DashboardLayout>
        );
    }
    return <VHostsCockpit />;
}

function VHostsCockpit() {
    const [state, setState] = useState<VhostState | null>(null);
    const [sources, setSources] = useState<Source[]>([]);
    const [loading, setLoading] = useState(true);
    const [savingSource, setSavingSource] = useState(false);
    const [applying, setApplying] = useState<"reconcile" | "reload" | null>(null);
    const [result, setResult] = useState<ReconcileResult | null>(null);
    const [confirm, setConfirm] = useState<null | "reconcile" | "reload">(null);
    const [editHost, setEditHost] = useState<null | ManageRow>(null);
    const [editRedirect, setEditRedirect] = useState<null | ManageRow>(null);

    const loadState = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetch("/post/vhost/state", { cache: "no-store" });
            setState(res.ok ? await res.json() : { configured: false, vhostsDir: "", liveReload: false, error: await res.text() });
        } catch (err) {
            setState({ configured: false, vhostsDir: "", liveReload: false, error: String(err) });
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void fetch("/post/datasources", { cache: "no-store" })
            .then((r) => (r.ok ? r.json() : null))
            .then((d) => setSources((d?.dataSources as Source[]) ?? []))
            .catch(() => setSources([]));
        void loadState();
    }, [loadState]);

    const changeSource = async (name: string) => {
        setSavingSource(true);
        try {
            const res = await fetch("/post/settings", {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ key: "vhost_data_source", value: name }),
            });
            if (!res.ok) {
                toast.error(`Could not set host-source: ${(await res.text()).trim() || res.statusText}`);
                return;
            }
            toast.success(name ? `Reading vhosts from "${name}"` : "Host-source cleared");
            setResult(null);
            await loadState();
        } catch (err) {
            toast.error(`Could not set host-source: ${String(err)}`);
        } finally {
            setSavingSource(false);
        }
    };

    const runApply = async (kind: "reconcile" | "reload") => {
        setConfirm(null);
        setApplying(kind);
        try {
            const res = await fetch(`/post/vhost/${kind}`, { method: "POST" });
            const data = (await res.json()) as ReconcileResult;
            setResult(data);
            if (data.error) {
                toast.error(data.reloaded ? "Applied with warnings" : summarizeError(data.error));
            } else if (data.reloaded) {
                toast.success(`Caddy reloaded — ${data.written.length} written, ${data.removed.length} removed`);
            } else {
                toast.success("Reconcile complete (no reload needed)");
            }
            await loadState();
        } catch (err) {
            toast.error(`Apply failed: ${String(err)}`);
        } finally {
            setApplying(null);
        }
    };

    const pruneOrphan = async (name: string) => {
        try {
            const res = await fetch("/post/vhost/orphan/prune", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ name }),
            });
            const data = (await res.json()) as ReconcileResult;
            setResult(data);
            if (data.error) {
                toast.error(summarizeError(data.error));
            } else {
                toast.success(`Pruned ${name}`);
            }
            await loadState();
        } catch (err) {
            toast.error(`Prune failed: ${String(err)}`);
        }
    };

    const deleteRow = async (kind: "system" | "redirect", row: ManageRow) => {
        if (!window.confirm(`Disable ${row.host}? It is soft-deleted in the database and removed from Caddy on the next reconcile.`)) return;
        try {
            const res = await fetch(`/post/vhost/${kind}?id=${row.id}`, { method: "DELETE" });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`${row.host} disabled`);
            await loadState();
        } catch (err) {
            toast.error(`Delete failed: ${String(err)}`);
        }
    };

    const dry = state?.dryRun;
    const manage = state?.manage;
    const live = state?.liveReload ?? false;
    const hostsOnDisk = dry?.files.length ?? 0;
    const pending = (dry?.would_write.length ?? 0) + (dry?.would_remove.length ?? 0);
    const orphanCount = dry?.orphans.length ?? 0;

    return (
        <DashboardLayout
            title="Caddy VHosts"
            description="Desired state is read from a data source and compared to the vhosts folder — validated before any reload, never applied blind."
            wide
            actions={
                <Button variant="outline" size="sm" className="gap-2" onClick={loadState} disabled={loading}>
                    <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                    Refresh
                </Button>
            }
        >
            <div className="space-y-4">
                {/* summary strip + host-source picker */}
                <div className="flex flex-wrap items-center gap-x-6 gap-y-4 rounded-md border border-border bg-card p-4">
                    <Stat n={hostsOnDisk} label="Hosts on disk" />
                    <span className="hidden h-8 w-px bg-border sm:block" />
                    <Stat n={pending} label="Pending changes" tone={pending > 0 ? "warn" : undefined} />
                    <span className="hidden h-8 w-px bg-border sm:block" />
                    <Stat n={orphanCount} label="Orphans" tone={orphanCount > 0 ? "err" : undefined} />
                    <span className="flex-1" />
                    {dry ? (
                        dry.in_sync ? (
                            <Pill tone="ok">In sync</Pill>
                        ) : (
                            <Pill tone="warn">Drift — {pending} pending</Pill>
                        )
                    ) : null}
                    <label className="inline-flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
                        <Database className="h-3.5 w-3.5" />
                        Reading from
                        <select
                            value={state?.source ?? ""}
                            onChange={(e) => changeSource(e.target.value)}
                            disabled={savingSource}
                            className="rounded border border-input bg-background px-2 py-1 text-xs text-foreground outline-none focus:ring-1 focus:ring-ring"
                        >
                            <option value="">— select —</option>
                            {sources.map((s) => (
                                <option key={s.id} value={s.name}>
                                    {s.name}
                                </option>
                            ))}
                        </select>
                    </label>
                </div>

                {loading ? (
                    <div className="flex min-h-48 items-center justify-center rounded-md border border-border bg-card">
                        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                    </div>
                ) : !state?.configured ? (
                    <EmptyBanner
                        title="No host-source selected"
                        body={
                            sources.length === 0
                                ? "Add a MySQL data source under Settings → Data Sources, then select it here."
                                : "Pick the data source that holds website_hosts / platform_hosts above."
                        }
                    />
                ) : state.error ? (
                    <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                        <span>{state.error}</span>
                    </div>
                ) : (
                    <>
                        {/* apply / reload controls */}
                        <ApplyBar
                            live={live}
                            pending={pending}
                            inSync={dry?.in_sync ?? true}
                            applying={applying}
                            onReconcile={() => setConfirm("reconcile")}
                            onReload={() => setConfirm("reload")}
                        />

                        {result ? <ResultPanel result={result} onDismiss={() => setResult(null)} /> : null}

                        {dry ? <HostTable hosts={dry.hosts} /> : null}

                        {/* management: system hosts + redirects */}
                        {manage ? (
                            <>
                                <ManageTable
                                    title="System hosts"
                                    subtitle="platform_hosts — panel-owned reverse proxies. Changes apply on the next reconcile."
                                    rows={manage.systemHosts}
                                    columns="system"
                                    onAdd={() => setEditHost({ id: 0, host: "", serverStack: manage.stacks[0] ?? "", target: "", isActive: true, softDeleted: false })}
                                    onEdit={(r) => setEditHost(r)}
                                    onDelete={(r) => deleteRow("system", r)}
                                />
                                <ManageTable
                                    title="Redirects"
                                    subtitle="platform_redirect_hosts — host → URL redirects."
                                    rows={manage.redirects}
                                    columns="redirect"
                                    onAdd={() => setEditRedirect({ id: 0, host: "", target: "", code: 301, isActive: true, softDeleted: false })}
                                    onEdit={(r) => setEditRedirect(r)}
                                    onDelete={(r) => deleteRow("redirect", r)}
                                />
                            </>
                        ) : null}

                        {/* orphans triage */}
                        {dry && dry.orphans.length > 0 ? <OrphansPanel orphans={dry.orphans} live={live} onPrune={pruneOrphan} /> : null}

                        {dry?.skips && dry.skips.length > 0 ? <Skips skips={dry.skips} /> : null}
                    </>
                )}
            </div>

            {confirm ? (
                <ConfirmApply
                    kind={confirm}
                    live={live}
                    dry={dry}
                    onCancel={() => setConfirm(null)}
                    onConfirm={() => runApply(confirm)}
                />
            ) : null}

            {editHost ? (
                <HostForm
                    row={editHost}
                    stacks={manage?.stacks ?? []}
                    onClose={() => setEditHost(null)}
                    onSaved={async () => {
                        setEditHost(null);
                        await loadState();
                    }}
                />
            ) : null}

            {editRedirect ? (
                <RedirectForm
                    row={editRedirect}
                    onClose={() => setEditRedirect(null)}
                    onSaved={async () => {
                        setEditRedirect(null);
                        await loadState();
                    }}
                />
            ) : null}
        </DashboardLayout>
    );
}

function ApplyBar({
    live,
    pending,
    inSync,
    applying,
    onReconcile,
    onReload,
}: {
    live: boolean;
    pending: number;
    inSync: boolean;
    applying: "reconcile" | "reload" | null;
    onReconcile: () => void;
    onReload: () => void;
}) {
    return (
        <div
            className={`flex flex-wrap items-center gap-3 rounded-md border px-4 py-3 ${
                live ? "border-emerald-500/25 bg-emerald-500/[0.05]" : "border-amber-500/25 bg-amber-500/[0.05]"
            }`}
        >
            {live ? (
                <ShieldCheck className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
            ) : (
                <Lock className="h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
            )}
            <div className="min-w-0 flex-1">
                <p className="text-xs font-semibold text-foreground">
                    {live ? "Live reconcile is armed" : "Live reconcile is gated"}
                </p>
                <p className="text-[11px] text-muted-foreground">
                    {live
                        ? "Apply renders files, validates with caddy adapt, then reloads Caddy via the admin API."
                        : "Edits save to the database now. Rendering + validated reload is off until an operator sets CADDY_LIVE_RELOAD=1 and restarts."}
                </p>
            </div>
            <Button variant="outline" size="sm" className="gap-2" onClick={onReload} disabled={applying !== null}>
                {applying === "reload" ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                Reload
            </Button>
            <Button size="sm" className="gap-2" onClick={onReconcile} disabled={applying !== null}>
                {applying === "reconcile" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-4 w-4" />}
                {inSync ? "Reconcile" : `Apply ${pending} change${pending === 1 ? "" : "s"}`}
            </Button>
        </div>
    );
}

function ConfirmApply({
    kind,
    live,
    dry,
    onCancel,
    onConfirm,
}: {
    kind: "reconcile" | "reload";
    live: boolean;
    dry?: DryRun;
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const writes = dry?.would_write ?? [];
    const removes = dry?.would_remove ?? [];
    return (
        <Modal onClose={onCancel} title={kind === "reload" ? "Re-validate & reload Caddy" : "Reconcile & reload Caddy"}>
            {!live ? (
                <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    <Lock className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>
                        Live reconcile is <b>not enabled</b> on this server. This will return the gate message without touching Caddy —
                        safe to run to verify the gate.
                    </span>
                </div>
            ) : null}
            <p className="text-xs text-muted-foreground">
                {kind === "reload"
                    ? "Re-adapts the current folder and reloads Caddy — no files are written or removed."
                    : "Renders desired-state files, validates the full config with caddy adapt, backs up the prior folder, then reloads via the admin API. The dashboard domains are asserted present before any reload."}
            </p>
            {kind === "reconcile" ? (
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    <DiffList tone="warn" label="Will write" items={writes} />
                    <DiffList tone="err" label="Will remove" items={removes} />
                </div>
            ) : null}
            <div className="mt-5 flex justify-end gap-2">
                <Button variant="outline" size="sm" onClick={onCancel}>
                    Cancel
                </Button>
                <Button size="sm" className="gap-2" onClick={onConfirm}>
                    <Zap className="h-4 w-4" />
                    {kind === "reload" ? "Reload now" : "Apply & reload"}
                </Button>
            </div>
        </Modal>
    );
}

function DiffList({ tone, label, items }: { tone: "warn" | "err"; label: string; items: string[] }) {
    const color = tone === "warn" ? "text-amber-600 dark:text-amber-400" : "text-destructive";
    return (
        <div className="rounded-md border border-border bg-muted/30 p-3">
            <p className={`text-[11px] font-semibold uppercase tracking-wide ${color}`}>
                {label} ({items.length})
            </p>
            {items.length === 0 ? (
                <p className="mt-1 text-[11px] text-muted-foreground">None</p>
            ) : (
                <ul className="mt-1 max-h-32 space-y-0.5 overflow-y-auto">
                    {items.map((i) => (
                        <li key={i} className="truncate font-mono text-[11px] text-foreground">
                            {i}
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

function ResultPanel({ result, onDismiss }: { result: ReconcileResult; onDismiss: () => void }) {
    const ok = !result.error;
    return (
        <div
            className={`rounded-md border p-4 ${
                ok ? "border-emerald-500/25 bg-emerald-500/[0.05]" : "border-destructive/30 bg-destructive/[0.06]"
            }`}
        >
            <div className="flex items-start gap-2">
                {ok ? (
                    <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                ) : (
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                )}
                <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-foreground">
                        {result.reloaded ? "Caddy reloaded" : ok ? "Reconcile complete — no reload" : "Not applied"}
                        <span className="ml-2 text-[11px] font-normal text-muted-foreground">{result.duration_ms}ms</span>
                    </p>
                    <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
                        <span>
                            reloaded: <b className="text-foreground">{String(result.reloaded)}</b>
                        </span>
                        <span>
                            written: <b className="text-foreground">{result.written.length}</b>
                        </span>
                        <span>
                            removed: <b className="text-foreground">{result.removed.length}</b>
                        </span>
                        {result.removes_suppressed && result.removes_suppressed.length > 0 ? (
                            <span>
                                removes suppressed (first pass): <b className="text-foreground">{result.removes_suppressed.length}</b>
                            </span>
                        ) : null}
                        <span>
                            orphans: <b className="text-foreground">{result.orphans.length}</b>
                        </span>
                    </div>
                    {result.error ? <p className="mt-2 font-mono text-[11px] text-destructive">{result.error}</p> : null}
                    {result.adapt_warnings && result.adapt_warnings.length > 0 ? (
                        <details className="mt-2">
                            <summary className="cursor-pointer text-[11px] text-amber-600 dark:text-amber-400">
                                {result.adapt_warnings.length} adapt warning(s)
                            </summary>
                            <ul className="mt-1 space-y-0.5">
                                {result.adapt_warnings.map((w, i) => (
                                    <li key={i} className="font-mono text-[11px] text-muted-foreground">
                                        {w}
                                    </li>
                                ))}
                            </ul>
                        </details>
                    ) : null}
                    {result.backup_path ? (
                        <p className="mt-1 font-mono text-[11px] text-muted-foreground">backup: {result.backup_path}</p>
                    ) : null}
                </div>
                <button onClick={onDismiss} className="text-[11px] text-muted-foreground hover:text-foreground">
                    Dismiss
                </button>
            </div>
        </div>
    );
}

function ManageTable({
    title,
    subtitle,
    rows,
    columns,
    onAdd,
    onEdit,
    onDelete,
}: {
    title: string;
    subtitle: string;
    rows: ManageRow[];
    columns: "system" | "redirect";
    onAdd: () => void;
    onEdit: (r: ManageRow) => void;
    onDelete: (r: ManageRow) => void;
}) {
    return (
        <div className="overflow-hidden rounded-md border border-border bg-card">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
                <div>
                    <p className="text-sm font-semibold text-foreground">{title}</p>
                    <p className="text-[11px] text-muted-foreground">{subtitle}</p>
                </div>
                <Button variant="outline" size="sm" className="gap-1.5" onClick={onAdd}>
                    <Plus className="h-3.5 w-3.5" />
                    Add
                </Button>
            </div>
            {rows.length === 0 ? (
                <p className="px-4 py-6 text-center text-xs text-muted-foreground">No rows.</p>
            ) : (
                <div className="overflow-x-auto">
                    <table className="w-full min-w-[680px] text-left text-xs">
                        <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                            <tr>
                                <th className="px-4 py-2.5 font-medium">Host</th>
                                {columns === "system" ? <th className="px-4 py-2.5 font-medium">Stack</th> : null}
                                <th className="px-4 py-2.5 font-medium">{columns === "system" ? "Upstream" : "Target URL"}</th>
                                {columns === "redirect" ? <th className="px-4 py-2.5 font-medium">Code</th> : null}
                                <th className="px-4 py-2.5 font-medium">State</th>
                                <th className="px-4 py-2.5 text-right font-medium">Actions</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                            {rows.map((r) => (
                                <tr key={r.id} className={r.isActive ? "" : "opacity-55"}>
                                    <td className="px-4 py-2.5 font-mono text-foreground">{r.host}</td>
                                    {columns === "system" ? (
                                        <td className="px-4 py-2.5">
                                            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                                                {r.serverStack || "—"}
                                            </span>
                                        </td>
                                    ) : null}
                                    <td className="px-4 py-2.5 font-mono text-muted-foreground">{r.target}</td>
                                    {columns === "redirect" ? <td className="px-4 py-2.5 tabular-nums text-muted-foreground">{r.code}</td> : null}
                                    <td className="px-4 py-2.5">
                                        {r.isActive ? <Pill tone="ok">Active</Pill> : <Pill tone="warn">Disabled</Pill>}
                                    </td>
                                    <td className="px-4 py-2.5">
                                        <div className="flex justify-end gap-1">
                                            <button
                                                onClick={() => onEdit(r)}
                                                className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                                                title="Edit"
                                            >
                                                <Pencil className="h-3.5 w-3.5" />
                                            </button>
                                            <button
                                                onClick={() => onDelete(r)}
                                                className="rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                                                title="Disable"
                                            >
                                                <Trash2 className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

function HostForm({ row, stacks, onClose, onSaved }: { row: ManageRow; stacks: string[]; onClose: () => void; onSaved: () => void }) {
    const [host, setHost] = useState(row.host);
    const [serverStack, setServerStack] = useState(row.serverStack ?? stacks[0] ?? "");
    const [target, setTarget] = useState(row.target);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);

    const save = async () => {
        setSaving(true);
        try {
            const res = await fetch("/post/vhost/system", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), serverStack, target: target.trim(), isActive }),
            });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`${host.trim()} saved`);
            onSaved();
        } catch (err) {
            toast.error(`Save failed: ${String(err)}`);
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal onClose={onClose} title={row.id === 0 ? "Add system host" : "Edit system host"}>
            <div className="space-y-3">
                <Field label="Hostname">
                    <input
                        value={host}
                        onChange={(e) => setHost(e.target.value)}
                        placeholder="app.example.com"
                        className={inputCls}
                        autoFocus
                    />
                </Field>
                <Field label="Server stack" hint="Upstream port is derived from the stack — only known stacks are offered.">
                    <select value={serverStack} onChange={(e) => setServerStack(e.target.value)} className={inputCls}>
                        {stacks.map((s) => (
                            <option key={s} value={s}>
                                {s}
                            </option>
                        ))}
                    </select>
                </Field>
                <Field label="Target" hint="Upstream host:port the reverse_proxy dials.">
                    <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="127.0.0.1:8005" className={inputCls} />
                </Field>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                    Active (rendered to a vhost file)
                </label>
            </div>
            <FormActions saving={saving} onCancel={onClose} onSave={save} disabled={!host.trim() || !target.trim()} />
        </Modal>
    );
}

function RedirectForm({ row, onClose, onSaved }: { row: ManageRow; onClose: () => void; onSaved: () => void }) {
    const [host, setHost] = useState(row.host);
    const [target, setTarget] = useState(row.target);
    const [code, setCode] = useState(row.code ?? 301);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);

    const save = async () => {
        setSaving(true);
        try {
            const res = await fetch("/post/vhost/redirect", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), target: target.trim(), code, isActive }),
            });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`${host.trim()} saved`);
            onSaved();
        } catch (err) {
            toast.error(`Save failed: ${String(err)}`);
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal onClose={onClose} title={row.id === 0 ? "Add redirect" : "Edit redirect"}>
            <div className="space-y-3">
                <Field label="Hostname">
                    <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="old.example.com" className={inputCls} autoFocus />
                </Field>
                <Field label="Target URL">
                    <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="https://new.example.com" className={inputCls} />
                </Field>
                <Field label="Status code">
                    <select value={code} onChange={(e) => setCode(Number(e.target.value))} className={inputCls}>
                        <option value={301}>301 — Permanent</option>
                        <option value={302}>302 — Found (temporary)</option>
                        <option value={307}>307 — Temporary (keep method)</option>
                        <option value={308}>308 — Permanent (keep method)</option>
                    </select>
                </Field>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                    Active
                </label>
            </div>
            <FormActions saving={saving} onCancel={onClose} onSave={save} disabled={!host.trim() || !target.trim()} />
        </Modal>
    );
}

function OrphansPanel({ orphans, live, onPrune }: { orphans: string[]; live: boolean; onPrune: (name: string) => void }) {
    return (
        <div className="overflow-hidden rounded-md border border-destructive/25 bg-destructive/[0.03]">
            <div className="border-b border-destructive/20 px-4 py-3">
                <p className="text-sm font-semibold text-foreground">
                    {orphans.length} orphan file{orphans.length === 1 ? "" : "s"}
                </p>
                <p className="text-[11px] text-muted-foreground">
                    On disk with no active database row. Never auto-removed — prune each one deliberately (protected/wildcard files are refused).
                </p>
            </div>
            <ul className="divide-y divide-border">
                {orphans.map((name) => (
                    <li key={name} className="flex items-center justify-between px-4 py-2.5">
                        <span className="font-mono text-xs text-foreground">{name}</span>
                        <Button
                            variant="outline"
                            size="sm"
                            className="gap-1.5 text-destructive hover:bg-destructive/10"
                            onClick={() => onPrune(name)}
                            title={live ? "Remove file and reload" : "Gated — will return the gate message"}
                        >
                            <Trash2 className="h-3.5 w-3.5" />
                            Prune
                        </Button>
                    </li>
                ))}
            </ul>
        </div>
    );
}

function HostTable({ hosts }: { hosts: HostRow[] }) {
    if (hosts.length === 0) {
        return <EmptyBanner title="No managed hosts" body="No active host rows in the selected data source, and no drift on disk." />;
    }
    return (
        <div className="overflow-hidden rounded-md border border-border bg-card">
            <div className="border-b border-border px-4 py-3">
                <p className="text-sm font-semibold text-foreground">Drift</p>
                <p className="text-[11px] text-muted-foreground">Desired state (all sources) vs the vhosts folder.</p>
            </div>
            <div className="overflow-x-auto">
                <table className="w-full min-w-[760px] text-left text-xs">
                    <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                        <tr>
                            <th className="px-4 py-3 font-medium">Hostname</th>
                            <th className="px-4 py-3 font-medium">Kind</th>
                            <th className="px-4 py-3 font-medium">Upstream</th>
                            <th className="px-4 py-3 font-medium">Status</th>
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                        {hosts.map((h, i) => (
                            <tr key={`${h.hostname}-${i}`} className={rowTint(h.status)}>
                                <td className="px-4 py-3 font-mono text-foreground">{h.hostname}</td>
                                <td className="px-4 py-3">
                                    <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                                        {h.kind === "orphan" ? "orphan · no DB row" : h.stack ? `${h.kind} · ${h.stack}` : h.kind}
                                    </span>
                                </td>
                                <td className="px-4 py-3 font-mono text-muted-foreground">
                                    {h.upstream ? `→ ${h.upstream}` : h.status === "will_remove" ? "— disabled in DB" : "on disk only"}
                                </td>
                                <td className="px-4 py-3">
                                    <StatusChip status={h.status} />
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

function Skips({ skips }: { skips: Skip[] }) {
    return (
        <div className="rounded-md border border-amber-500/25 bg-amber-500/5 p-4">
            <p className="text-xs font-semibold text-amber-700 dark:text-amber-300">
                {skips.length} row{skips.length === 1 ? "" : "s"} skipped
            </p>
            <ul className="mt-2 space-y-1 text-xs text-muted-foreground">
                {skips.map((s, i) => (
                    <li key={i}>
                        <span className="font-mono text-foreground">{s.host}</span> — {s.reason}
                    </li>
                ))}
            </ul>
        </div>
    );
}

function Modal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
            <div
                className="w-full max-w-md rounded-lg border border-border bg-card p-5 shadow-xl"
                onClick={(e) => e.stopPropagation()}
            >
                <h2 className="mb-3 text-base font-semibold text-foreground">{title}</h2>
                {children}
            </div>
        </div>
    );
}

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
    return (
        <label className="block">
            <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>
            {children}
            {hint ? <span className="mt-1 block text-[11px] text-muted-foreground">{hint}</span> : null}
        </label>
    );
}

function FormActions({ saving, disabled, onCancel, onSave }: { saving: boolean; disabled?: boolean; onCancel: () => void; onSave: () => void }) {
    return (
        <div className="mt-5 flex justify-end gap-2">
            <Button variant="outline" size="sm" onClick={onCancel} disabled={saving}>
                Cancel
            </Button>
            <Button size="sm" className="gap-2" onClick={onSave} disabled={saving || disabled}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                Save
            </Button>
        </div>
    );
}

function Stat({ n, label, tone }: { n: number; label: string; tone?: "warn" | "err" }) {
    const color = tone === "warn" ? "text-amber-500" : tone === "err" ? "text-destructive" : "text-foreground";
    return (
        <div className="flex flex-col gap-0.5">
            <span className={`text-xl font-bold tabular-nums ${color}`}>{n}</span>
            <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{label}</span>
        </div>
    );
}

function StatusChip({ status }: { status: string }) {
    const map: Record<string, { tone: "ok" | "warn" | "err"; label: string }> = {
        in_sync: { tone: "ok", label: "In sync" },
        will_write: { tone: "warn", label: "Will write" },
        will_remove: { tone: "err", label: "Will remove" },
        orphan: { tone: "err", label: "Orphan" },
    };
    const s = map[status] ?? { tone: "warn" as const, label: status };
    return <Pill tone={s.tone}>{s.label}</Pill>;
}

function Pill({ tone, children }: { tone: "ok" | "warn" | "err"; children: ReactNode }) {
    const cls =
        tone === "ok"
            ? "text-emerald-600 dark:text-emerald-400 border-emerald-500/20 bg-emerald-500/10"
            : tone === "warn"
              ? "text-amber-600 dark:text-amber-400 border-amber-500/25 bg-amber-500/10"
              : "text-destructive border-destructive/20 bg-destructive/10";
    return (
        <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${cls}`}>
            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
            {children}
        </span>
    );
}

function rowTint(status: string): string {
    if (status === "orphan" || status === "will_remove") return "bg-destructive/[0.04]";
    if (status === "will_write") return "bg-amber-500/[0.05]";
    return "";
}

function EmptyBanner({ title, body }: { title: string; body: string }) {
    return (
        <div className="flex min-h-40 flex-col items-center justify-center rounded-md border border-dashed border-border px-6 text-center">
            <Database className="mb-3 h-8 w-8 text-muted-foreground/40" />
            <p className="text-sm font-medium text-foreground">{title}</p>
            <p className="mt-1 max-w-md text-xs text-muted-foreground">{body}</p>
        </div>
    );
}

function summarizeError(err: string): string {
    const first = err.split("\n")[0].trim();
    return first.length > 140 ? `${first.slice(0, 137)}…` : first;
}

const inputCls =
    "w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring";
