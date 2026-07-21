import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { EmptyBanner, Field, FormActions, inputCls, type ManageRow, Modal, Pill, type Upstream, ViewHeader } from "./shared";

const CUSTOM = "__custom__";

// SystemView manages platform_hosts — panel-owned reverse proxies to ANY running
// container (not just the code stacks). Full CRUD; live on the next global reconcile.
export default function SystemView({ rows, upstreams, onSaved }: { rows: ManageRow[]; upstreams: Upstream[]; onSaved: () => void }) {
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
                        onClick={() => setEdit({ id: 0, host: "", serverStack: "", target: "", isActive: true, softDeleted: false })}
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
                                    <th className="px-4 py-2.5 font-medium">Service</th>
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
                    upstreams={upstreams}
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

function HostForm({ row, upstreams, onClose, onSaved }: { row: ManageRow; upstreams: Upstream[]; onClose: () => void; onSaved: () => void }) {
    const [host, setHost] = useState(row.host);
    const matched = upstreams.find((u) => u.target === row.target);
    const [sel, setSel] = useState(row.target ? (matched ? row.target : CUSTOM) : (upstreams[0]?.target ?? CUSTOM));
    const [customTarget, setCustomTarget] = useState(matched ? "" : row.target);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);

    const target = sel === CUSTOM ? customTarget.trim() : sel;
    const serverStack = sel === CUSTOM ? "custom" : (upstreams.find((u) => u.target === sel)?.name ?? "system");

    const save = async () => {
        setSaving(true);
        try {
            const res = await fetch("/post/vhost/system", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), serverStack, target, isActive }),
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
                    <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="dbs.example.com" className={inputCls} autoFocus />
                </Field>
                <Field label="Upstream" hint="The running container this host reverse-proxies to (synced from this server), or a custom host:port.">
                    <select value={sel} onChange={(e) => setSel(e.target.value)} className={inputCls}>
                        {upstreams.map((u) => (
                            <option key={u.target} value={u.target}>
                                {u.name} — {u.target}
                            </option>
                        ))}
                        <option value={CUSTOM}>Custom host:port…</option>
                    </select>
                </Field>
                {sel === CUSTOM ? (
                    <Field label="Custom target" hint="Upstream host:port the reverse_proxy dials.">
                        <input value={customTarget} onChange={(e) => setCustomTarget(e.target.value)} placeholder="127.0.0.1:9001" className={inputCls} />
                    </Field>
                ) : null}
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                    Active (rendered to a vhost file)
                </label>
            </div>
            <FormActions saving={saving} onCancel={onClose} onSave={save} disabled={!host.trim() || !target} />
        </Modal>
    );
}
