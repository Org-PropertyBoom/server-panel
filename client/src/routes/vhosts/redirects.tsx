import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { EmptyBanner, Field, FormActions, HostLink, inputCls, type ManageRow, Modal, Pill, UrlLink, ViewHeader } from "./shared";

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
