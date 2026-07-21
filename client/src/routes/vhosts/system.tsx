import { useState } from "react";
import { AlertTriangle, Lock, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { Field, FormActions, HostLink, inputCls, type ManageRow, Modal, Pill, type PinnedRow, type Upstream, ViewHeader } from "./shared";

// SystemView manages platform_hosts — panel-owned reverse proxies to ANY running
// container (not just the code stacks). Full CRUD; live on the next global reconcile.
// The pinned domains (derived from the ACTUAL Caddyfile) show as read-only rows on
// top — they ARE App/System hosts, just static blocks, not DB-reconciled — with a
// drift flag vs what the reload actually guards.
export default function SystemView({ rows, upstreams, pinned, pinnedWarning, onSaved }: { rows: ManageRow[]; upstreams: Upstream[]; pinned: PinnedRow[]; pinnedWarning?: string; onSaved: () => void }) {
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
            {pinnedWarning ? (
                <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>{pinnedWarning}</span>
                </div>
            ) : null}
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
                            {pinned.map((p) => (
                                <tr key={`pinned-${p.host}`} className={p.drift === "missing" ? "bg-destructive/[0.06]" : "bg-primary/[0.04]"}>
                                    <td className="px-4 py-2.5 font-mono text-foreground">
                                        <span className="inline-flex items-center gap-1.5">
                                            <Lock className="h-3 w-3 text-muted-foreground" />
                                            <HostLink host={p.host} />
                                        </span>
                                    </td>
                                    <td className="px-4 py-2.5">
                                        <div className="flex flex-wrap items-center gap-1.5">
                                            {p.role ? <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">{p.role}</span> : null}
                                            <span className="rounded-full border border-primary/20 bg-primary/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
                                                pinned
                                            </span>
                                        </div>
                                    </td>
                                    <td className="px-4 py-2.5 font-mono text-muted-foreground">
                                        {p.upstreams && p.upstreams.length > 0 ? p.upstreams.map((u) => `→ ${u}`).join(", ") : "static · main Caddyfile"}
                                    </td>
                                    <td className="px-4 py-2.5">
                                        {p.drift === "missing" ? (
                                            <Pill tone="err">Guarded, not pinned</Pill>
                                        ) : p.drift === "unmanaged" ? (
                                            <Pill tone="warn">Pinned · unmanaged</Pill>
                                        ) : (
                                            <Pill tone="ok">Protected</Pill>
                                        )}
                                    </td>
                                    <td className="px-4 py-2.5 text-right">
                                        <Lock className="ml-auto h-3.5 w-3.5 text-muted-foreground/40" aria-label="Read-only (static Caddyfile block)" />
                                    </td>
                                </tr>
                            ))}
                            {rows.map((r) => (
                                <tr key={r.id} className={r.isActive ? "" : "opacity-55"}>
                                    <td className="px-4 py-2.5 font-mono text-foreground">
                                        <HostLink host={r.host} />
                                    </td>
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
                            {rows.length === 0 ? (
                                <tr>
                                    <td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">
                                        No editable system hosts yet — use “Add system host”.
                                    </td>
                                </tr>
                            ) : null}
                        </tbody>
                    </table>
                </div>
            </div>
            {pinned.length > 0 ? (
                <p className="mt-2 text-[11px] text-muted-foreground">
                    <Lock className="mr-1 inline h-3 w-3" />
                    Pinned rows are derived from the actual main Caddyfile — read-only static blocks, not DB-reconciled.{" "}
                    <b className="text-destructive">Guarded, not pinned</b> means a domain the reload asserts but isn’t really a static block (fix the Caddyfile);{" "}
                    <b className="text-amber-600 dark:text-amber-400">Pinned · unmanaged</b> means a static block the reload doesn’t guard.
                </p>
            ) : null}
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
    const [target, setTarget] = useState(row.target);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);
    const [showSuggest, setShowSuggest] = useState(false);

    // Backend combobox: type a host:port OR pick a running container (port auto-fills).
    const q = target.toLowerCase().trim();
    const suggestions = upstreams
        .filter((u) => q === "" || u.target.toLowerCase().includes(q) || u.name.toLowerCase().includes(q))
        .slice(0, 8);

    const save = async () => {
        setSaving(true);
        try {
            const t = target.trim();
            // Label the service from a matching container, else a generic "custom" (a
            // host-level backend like server-panel :2205). platform_hosts.target takes
            // any host:port; server_stack is just a label here.
            const serverStack = upstreams.find((u) => u.target === t)?.name ?? "custom";
            const res = await fetch("/post/vhost/system", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), serverStack, target: t, isActive }),
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
                <Field label="Backend" hint="Pick a running container (port auto-fills), or type a host:port for a host-level service (e.g. 127.0.0.1:2205).">
                    <div className="relative">
                        <input
                            value={target}
                            onChange={(e) => {
                                setTarget(e.target.value);
                                setShowSuggest(true);
                            }}
                            onFocus={() => setShowSuggest(true)}
                            onBlur={() => window.setTimeout(() => setShowSuggest(false), 120)}
                            placeholder="pick a container or type 127.0.0.1:9001"
                            className={inputCls}
                            autoComplete="off"
                        />
                        {showSuggest && suggestions.length > 0 ? (
                            <ul className="absolute z-10 mt-1 max-h-56 w-full overflow-y-auto rounded-md border border-border bg-card shadow-lg">
                                {suggestions.map((u) => (
                                    <li key={u.target}>
                                        <button
                                            type="button"
                                            onMouseDown={(e) => {
                                                e.preventDefault();
                                                setTarget(u.target);
                                                setShowSuggest(false);
                                            }}
                                            className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-xs hover:bg-muted"
                                        >
                                            <span className="font-medium text-foreground">{u.name}</span>
                                            <span className="font-mono text-[11px] text-muted-foreground">{u.target}</span>
                                        </button>
                                    </li>
                                ))}
                            </ul>
                        ) : null}
                    </div>
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
