import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { EmptyBanner, Field, FormActions, inputCls, type ManageRow, Modal, Pill, ViewHeader } from "./shared";

// SystemView manages platform_hosts — panel-owned reverse proxies. Full CRUD; the
// changes become live on the next global reconcile.
export default function SystemView({ rows, stacks, onSaved }: { rows: ManageRow[]; stacks: string[]; onSaved: () => void }) {
    const [edit, setEdit] = useState<ManageRow | null>(null);

    const del = async (row: ManageRow) => {
        if (!window.confirm(`Disable ${row.host}? It is soft-deleted in the database and removed from Caddy on the next reconcile.`)) return;
        try {
            const res = await fetch(`/post/vhost/system?id=${row.id}`, { method: "DELETE" });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`${row.host} disabled`);
            onSaved();
        } catch (err) {
            toast.error(`Delete failed: ${String(err)}`);
        }
    };

    return (
        <div>
            <ViewHeader
                title="System hosts"
                subtitle="platform_hosts — panel-owned reverse proxies. Edits save to the database; they go live on the next reconcile."
                actions={
                    <Button
                        variant="outline"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setEdit({ id: 0, host: "", serverStack: stacks[0] ?? "", target: "", isActive: true, softDeleted: false })}
                    >
                        <Plus className="h-3.5 w-3.5" />
                        Add system host
                    </Button>
                }
            />
            {rows.length === 0 ? (
                <EmptyBanner title="No system hosts" body="Add a platform_hosts row to reverse-proxy a panel-owned domain to an upstream." />
            ) : (
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[680px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-2.5 font-medium">Host</th>
                                    <th className="px-4 py-2.5 font-medium">Stack</th>
                                    <th className="px-4 py-2.5 font-medium">Upstream</th>
                                    <th className="px-4 py-2.5 font-medium">State</th>
                                    <th className="px-4 py-2.5 text-right font-medium">Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {rows.map((r) => (
                                    <tr key={r.id} className={r.isActive ? "" : "opacity-55"}>
                                        <td className="px-4 py-2.5 font-mono text-foreground">{r.host}</td>
                                        <td className="px-4 py-2.5">
                                            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                                                {r.serverStack || "—"}
                                            </span>
                                        </td>
                                        <td className="px-4 py-2.5 font-mono text-muted-foreground">{r.target}</td>
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
                <HostForm
                    row={edit}
                    stacks={stacks}
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
                    <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="app.example.com" className={inputCls} autoFocus />
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
