import { useMemo, useState } from "react";
import { ExternalLink, Lock, Search } from "lucide-react";

import { EmptyBanner, HostLink, type HostHealth, type HostRow, rowTint, StatusChip, UnreachableChip, ViewHeader } from "./shared";

// Stack dashboard host per server_stack — the deep-link target for "Manage in
// stack" (tenant rows are stack-owned; edits/deletes happen there, not here).
const STACK_DASHBOARD: Record<string, string> = {
    phalcon: "app.propertyboom.co",
    laravel: "la-app.propertyboom.co",
    golang: "go-app.propertyboom.co",
};

function manageInStackUrl(host: string, stack?: string): string | null {
    const dash = stack ? STACK_DASHBOARD[stack] : undefined;
    if (!dash) return null;
    return `https://${dash}/dashboard/website-hosts?search=${encodeURIComponent(host)}`;
}

// TenantView renders website_hosts — the stack-owned tenant sites. READ-ONLY here:
// the stack apps own these rows; the panel only views + monitors drift + health.
export default function TenantView({ hosts, health }: { hosts: HostRow[]; health: Record<string, HostHealth> }) {
    const [stack, setStack] = useState<string>("");
    const [query, setQuery] = useState("");
    const [unreachableOnly, setUnreachableOnly] = useState(false);

    const stacks = useMemo(() => {
        const counts = new Map<string, number>();
        for (const h of hosts) {
            const s = h.stack || "—";
            counts.set(s, (counts.get(s) ?? 0) + 1);
        }
        return Array.from(counts.entries()).sort((a, b) => a[0].localeCompare(b[0]));
    }, [hosts]);

    const unreachableCount = useMemo(() => hosts.filter((h) => health[h.hostname]?.alert).length, [hosts, health]);

    const q = query.trim().toLowerCase();
    const filtered = hosts.filter(
        (h) =>
            (stack === "" || (h.stack || "—") === stack) &&
            (q === "" || h.hostname.toLowerCase().includes(q)) &&
            (!unreachableOnly || !!health[h.hostname]?.alert),
    );

    return (
        <div>
            <ViewHeader
                title="Tenant hosts"
                subtitle="website_hosts — tenant sites, owned by the stack apps. Read-only here; add/edit/delete via 'Manage in stack'. Drift is applied by the global reconcile."
                actions={
                    <span className="inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2.5 py-1 text-[11px] text-muted-foreground">
                        <Lock className="h-3.5 w-3.5" />
                        Read-only (stack-owned)
                    </span>
                }
            />

            {hosts.length === 0 ? (
                <EmptyBanner title="No tenant hosts" body="No active website_hosts rows in the selected data source, and no tenant drift on disk." />
            ) : (
                <>
                    {/* filter toolbar: stack chips + host search + unreachable toggle */}
                    <div className="mb-3 flex flex-wrap items-center gap-2">
                        <Chip label="All" count={hosts.length} active={stack === ""} onClick={() => setStack("")} />
                        {stacks.map(([s, n]) => (
                            <Chip key={s} label={s} count={n} active={stack === s} onClick={() => setStack(s)} />
                        ))}
                        <span className="flex-1" />
                        {unreachableCount > 0 ? (
                            <button
                                onClick={() => setUnreachableOnly((v) => !v)}
                                className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${
                                    unreachableOnly
                                        ? "border-amber-500/40 bg-amber-500/15 text-amber-600 dark:text-amber-400"
                                        : "border-border text-muted-foreground hover:bg-muted"
                                }`}
                                title="Show only hosts that are not reaching this server"
                            >
                                <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                                Unreachable ({unreachableCount})
                            </button>
                        ) : null}
                        <div className="relative">
                            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                            <input
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                placeholder="Filter hosts…"
                                className="h-8 w-48 rounded-md border border-input bg-background pl-8 pr-2 text-xs text-foreground outline-none focus:ring-1 focus:ring-ring"
                            />
                        </div>
                    </div>

                    {filtered.length === 0 ? (
                        <div className="rounded-md border border-dashed border-border px-6 py-10 text-center text-xs text-muted-foreground">
                            No hosts match the current filter.
                        </div>
                    ) : (
                        <div className="overflow-hidden rounded-md border border-border bg-card">
                            <div className="overflow-x-auto">
                                <table className="w-full min-w-[720px] text-left text-xs">
                                    <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                        <tr>
                                            <th className="px-4 py-3 font-medium">Hostname</th>
                                            <th className="px-4 py-3 font-medium">Stack</th>
                                            <th className="px-4 py-3 font-medium">Upstream</th>
                                            <th className="px-4 py-3 font-medium">Status</th>
                                            <th className="px-4 py-3 text-right font-medium">Manage</th>
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y divide-border">
                                        {filtered.map((h, i) => (
                                            <tr key={`${h.hostname}-${i}`} className={rowTint(h.status)}>
                                                <td className="px-4 py-3 font-mono text-foreground">
                                                    <HostLink host={h.hostname} />
                                                </td>
                                                <td className="px-4 py-3">
                                                    <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                                                        {h.stack || "—"}
                                                    </span>
                                                </td>
                                                <td className="px-4 py-3 font-mono text-muted-foreground">
                                                    {h.upstream ? `→ ${h.upstream}` : h.status === "will_remove" ? "— disabled in DB" : "on disk only"}
                                                </td>
                                                <td className="px-4 py-3">
                                                    <div className="flex flex-wrap items-center gap-1.5">
                                                        <StatusChip status={h.status} />
                                                        <UnreachableChip health={health[h.hostname]} />
                                                    </div>
                                                </td>
                                                <td className="px-4 py-3 text-right">
                                                    {manageInStackUrl(h.hostname, h.stack) ? (
                                                        <a
                                                            href={manageInStackUrl(h.hostname, h.stack) as string}
                                                            target="_blank"
                                                            rel="noopener noreferrer"
                                                            className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium text-muted-foreground hover:bg-muted hover:text-primary"
                                                            title={`Open ${h.hostname} in the ${h.stack} dashboard to add/edit/delete (tenant hosts are stack-owned)`}
                                                        >
                                                            Manage in stack
                                                            <ExternalLink className="h-3 w-3" />
                                                        </a>
                                                    ) : (
                                                        <span className="text-[11px] text-muted-foreground/50">—</span>
                                                    )}
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                            <div className="border-t border-border px-4 py-2 text-[11px] text-muted-foreground">
                                Showing {filtered.length} of {hosts.length}
                                {stack ? ` · stack: ${stack}` : ""}
                                {unreachableOnly ? " · unreachable only" : ""}
                            </div>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}

function Chip({ label, count, active, onClick }: { label: string; count: number; active: boolean; onClick: () => void }) {
    return (
        <button
            onClick={onClick}
            className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${
                active ? "border-primary/30 bg-primary/10 text-primary" : "border-border text-muted-foreground hover:bg-muted"
            }`}
        >
            {label}
            <span className={`tabular-nums ${active ? "text-primary/70" : "text-muted-foreground/70"}`}>{count}</span>
        </button>
    );
}
