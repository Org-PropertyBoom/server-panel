import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { EmptyBanner, HostLink, type ManageRow, Pill, UrlLink, ViewHeader } from "./shared";
import RedirectForm from "./redirect-form";

// RedirectsView manages platform_redirect_hosts — host → URL redirects answered at
// the edge. Full CRUD; live on the next global reconcile.
export default function RedirectsView({ rows, onSaved }: { rows: ManageRow[]; onSaved: () => void }) {
    const [edit, setEdit] = useState<ManageRow | null>(null);

    const del = async (row: ManageRow) => {
        if (!window.confirm(`Disable the redirect for ${row.host}? It is soft-deleted and removed from Caddy on the next reconcile.`)) return;
        try {
            const res = await fetch(`/post/vhost/redirect?id=${row.id}`, { method: "DELETE" });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`Redirect for ${row.host} disabled`);
            onSaved();
        } catch (err) {
            toast.error(`Delete failed: ${String(err)}`);
        }
    };

    return (
        <div>
            <ViewHeader
                title="Redirects"
                subtitle="platform_redirect_hosts — host → URL redirects. Edits save to the database; live on the next reconcile."
                actions={
                    <Button
                        variant="outline"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setEdit({ id: 0, host: "", target: "", code: 301, isActive: true, softDeleted: false })}
                    >
                        <Plus className="h-3.5 w-3.5" />
                        Add redirect
                    </Button>
                }
            />
            {rows.length === 0 ? (
                <EmptyBanner title="No redirects" body="Add a platform_redirect_hosts row to send a host to another URL at the edge." />
            ) : (
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[680px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-2.5 font-medium">Host</th>
                                    <th className="px-4 py-2.5 font-medium">Target URL</th>
                                    <th className="px-4 py-2.5 font-medium">Code</th>
                                    <th className="px-4 py-2.5 font-medium">State</th>
                                    <th className="px-4 py-2.5 text-right font-medium">Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {rows.map((r) => (
                                    <tr key={r.id} className={r.isActive ? "" : "opacity-55"}>
                                        <td className="px-4 py-2.5 font-mono text-foreground">
                                            <HostLink host={r.host} />
                                        </td>
                                        <td className="px-4 py-2.5 font-mono text-muted-foreground">
                                            <UrlLink url={r.target} />
                                        </td>
                                        <td className="px-4 py-2.5 tabular-nums text-muted-foreground">{r.code}</td>
                                        <td className="px-4 py-2.5">{r.isActive ? <Pill tone="ok">Active</Pill> : <Pill tone="warn">Disabled</Pill>}</td>
                                        <td className="px-4 py-2.5">
                                            <div className="flex justify-end gap-1">
                                                <button onClick={() => setEdit(r)} className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground" title="Edit">
                                                    <Pencil className="h-3.5 w-3.5" />
                                                </button>
                                                <button onClick={() => del(r)} className="rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" title="Disable">
                                                    <Trash2 className="h-3.5 w-3.5" />
                                                </button>
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}
            {edit ? (
                <RedirectForm
                    row={edit}
                    onClose={() => setEdit(null)}
                    onSaved={() => {
                        setEdit(null);
                        onSaved();
                    }}
                />
            ) : null}
        </div>
    );
}

