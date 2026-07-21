import { Lock } from "lucide-react";

import { EmptyBanner, type HostHealth, type HostRow, rowTint, StatusChip, UnreachableChip, ViewHeader } from "./shared";

// TenantView renders website_hosts — the stack-owned tenant sites. READ-ONLY here:
// the stack apps own these rows; the panel only views + monitors drift + health.
export default function TenantView({ hosts, health }: { hosts: HostRow[]; health: Record<string, HostHealth> }) {
    return (
        <div>
            <ViewHeader
                title="Tenant hosts"
                subtitle="website_hosts — tenant sites, owned by the stack apps. Read-only here; drift is applied by the global reconcile."
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
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[720px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-3 font-medium">Hostname</th>
                                    <th className="px-4 py-3 font-medium">Stack</th>
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
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}
        </div>
    );
}
