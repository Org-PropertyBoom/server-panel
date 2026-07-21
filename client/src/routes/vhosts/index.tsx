import { useCallback, useEffect, useState, type ReactNode } from "react";
import { AlertTriangle, Database, Loader2, Lock, RefreshCw } from "lucide-react";
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

type VhostState = {
    configured: boolean;
    source?: string;
    vhostsDir: string;
    message?: string;
    error?: string;
    dryRun?: DryRun;
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

    const loadState = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetch("/post/vhost/state", { cache: "no-store" });
            setState(res.ok ? await res.json() : { configured: false, vhostsDir: "", error: await res.text() });
        } catch (err) {
            setState({ configured: false, vhostsDir: "", error: String(err) });
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
            await loadState();
        } catch (err) {
            toast.error(`Could not set host-source: ${String(err)}`);
        } finally {
            setSavingSource(false);
        }
    };

    const dry = state?.dryRun;
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
                ) : dry ? (
                    <>
                        <HostTable hosts={dry.hosts} />
                        {dry.skips && dry.skips.length > 0 ? <Skips skips={dry.skips} /> : null}
                        <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
                            <Lock className="h-3.5 w-3.5 shrink-0" />
                            Read-only preview. Applying changes (write + validated reload) is a gated step — not yet enabled.
                        </div>
                    </>
                ) : null}
            </div>
        </DashboardLayout>
    );
}

function HostTable({ hosts }: { hosts: HostRow[] }) {
    if (hosts.length === 0) {
        return <EmptyBanner title="No managed hosts" body="No active host rows in the selected data source, and no drift on disk." />;
    }
    return (
        <div className="overflow-hidden rounded-md border border-border bg-card">
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
