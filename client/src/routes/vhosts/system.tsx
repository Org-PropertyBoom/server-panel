import { useState } from "react";
import { AlertTriangle, Loader2, Lock, Pencil, Pin, PinOff, Plus, Power, PowerOff, Shield, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { Field, FormActions, HostLink, inputCls, type ManageRow, Modal, type OriginCert, originCertFor, Pill, type PinnedRow, setHostTlsMode, suppressHost, summarizeError, type Upstream, ViewHeader } from "./shared";

// SystemView manages platform_hosts — panel-owned reverse proxies to ANY running
// container (not just the code stacks). Full CRUD; live on the next global reconcile.
// The pinned domains (derived from the ACTUAL Caddyfile) show as read-only rows on
// top — they ARE App/System hosts, just static blocks, not DB-reconciled — with a
// drift flag vs what the reload actually guards.
export default function SystemView({ rows, upstreams, pinned, pinnedWarning, originCerts, onSaved }: { rows: ManageRow[]; upstreams: Upstream[]; pinned: PinnedRow[]; pinnedWarning?: string; originCerts: OriginCert[]; onSaved: () => void }) {
    const [edit, setEdit] = useState<ManageRow | null>(null);
    const [removeBlock, setRemoveBlock] = useState<PinnedRow | null>(null);
    const [removing, setRemoving] = useState(false);
    const [pinRow, setPinRow] = useState<ManageRow | null>(null);
    const [unpinRow, setUnpinRow] = useState<PinnedRow | null>(null);
    const [converting, setConverting] = useState(false);
    const [suppressBusy, setSuppressBusy] = useState("");
    const [tlsRow, setTlsRow] = useState<ManageRow | null>(null); // TLS-mode confirm target
    const [tlsBusy, setTlsBusy] = useState("");
    const [originOpen, setOriginOpen] = useState(false);
    const originConfigured = originCerts.length > 0;

    // Switch a host between on-demand LE and a Cloudflare Origin cert (cf_origin),
    // then reconcile. cf_origin is for hosts proxied through Cloudflare. certPath/
    // keyPath are an optional per-host override (empty = the global default).
    const applyTlsMode = async (host: string, mode: string, certPath = "", keyPath = "") => {
        setTlsBusy(host);
        const res = await setHostTlsMode(host, mode, certPath, keyPath);
        if (res.error) toast.error(summarizeError(res.error));
        else if (res.reloaded) toast.success(`${host} → ${mode === "cf_origin" ? "Cloudflare Origin cert" : "on-demand LE"}`);
        else toast.error("TLS mode not applied");
        setTlsBusy("");
        setTlsRow(null);
        onSaved();
    };

    // Edge disable/enable (suppress) — served = Active AND not suppressed.
    const toggleSuppress = async (host: string, suppressed: boolean) => {
        setSuppressBusy(host);
        const res = await suppressHost(host, suppressed);
        if (res.error) toast.error(summarizeError(res.error));
        else if (res.reloaded) toast.success(`${host} ${suppressed ? "disabled" : "enabled"} at Caddy`);
        else toast.error(`Not ${suppressed ? "disabled" : "enabled"}`);
        setSuppressBusy("");
        onSaved();
    };

    // convert POSTs a pin/unpin and reports the truthful Result (both mutate the main
    // Caddyfile via the same backup → adapt → diff-assert → reload discipline).
    const convert = async (path: "pin" | "unpin", body: object, host: string, verb: string) => {
        setConverting(true);
        try {
            const res = await fetch(`/post/vhost/${path}`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(body),
            });
            const data = await res.json();
            if (data.error) {
                toast.error(summarizeError(String(data.error)));
            } else if (data.reloaded) {
                toast.success(`${host} ${verb}`);
            } else {
                toast.error(`Not ${verb}`);
            }
            setPinRow(null);
            setUnpinRow(null);
            onSaved();
        } catch (err) {
            toast.error(`${verb} failed: ${String(err)}`);
        } finally {
            setConverting(false);
        }
    };

    const removePinnedBlock = async (p: PinnedRow) => {
        setRemoving(true);
        try {
            const res = await fetch("/post/vhost/pinned/remove", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ host: p.host }),
            });
            const data = await res.json();
            if (data.error) {
                toast.error(summarizeError(String(data.error)));
            } else if (data.reloaded) {
                toast.success(`Removed ${p.host} from the Caddyfile`);
            } else {
                toast.error("Not removed");
            }
            setRemoveBlock(null);
            onSaved();
        } catch (err) {
            toast.error(`Remove failed: ${String(err)}`);
        } finally {
            setRemoving(false);
        }
    };

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
                    <div className="flex items-center gap-2">
                        <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setOriginOpen(true)} title="Registered Cloudflare Origin certificates (selected automatically by hostname)">
                            <Shield className={`h-3.5 w-3.5 ${originConfigured ? "text-emerald-600 dark:text-emerald-400" : "text-muted-foreground"}`} />
                            Origin certs{originConfigured ? ` (${originCerts.length})` : ""}
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            className="gap-1.5"
                            onClick={() => setEdit({ id: 0, host: "", serverStack: "", target: "", isActive: true, softDeleted: false })}
                        >
                            <Plus className="h-3.5 w-3.5" />
                            Add system host
                        </Button>
                    </div>
                }
            />
            {rows.some((r) => r.tlsMode === "cf_origin" && !r.tlsCertPath && !originCertFor(r.host, originCerts)) ? (
                <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>A host is set to <b>CF Origin</b> but no registered certificate covers it — those hosts are skipped at reconcile. Register the right zone's cert under <b>Origin certs</b>.</span>
                </div>
            ) : null}
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
                                <th className="px-4 py-2.5 font-medium">TLS</th>
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
                                    <td className="px-4 py-2.5 text-muted-foreground/50" title="Pinned static blocks manage their own TLS in the main Caddyfile">—</td>
                                    <td className="px-4 py-2.5 text-right">
                                        {p.drift === "unmanaged" ? (
                                            <div className="flex justify-end gap-1">
                                                <button
                                                    onClick={() => setUnpinRow(p)}
                                                    className="inline-flex items-center gap-1 rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                                                    title="Unpin — convert this static block back into a managed system host"
                                                >
                                                    <PinOff className="h-3.5 w-3.5" />
                                                </button>
                                                <button
                                                    onClick={() => setRemoveBlock(p)}
                                                    className="inline-flex items-center gap-1 rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                                                    title="Remove this stale static block from the main Caddyfile"
                                                >
                                                    <Trash2 className="h-3.5 w-3.5" />
                                                </button>
                                            </div>
                                        ) : (
                                            <Lock className="ml-auto h-3.5 w-3.5 text-muted-foreground/40" aria-label="Read-only (static Caddyfile block — pin-permanent)" />
                                        )}
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
                                    <td className="px-4 py-2.5">{r.suppressed ? <Pill tone="warn">Disabled · edge</Pill> : r.isActive ? <Pill tone="ok">Active</Pill> : <Pill tone="warn">Disabled</Pill>}</td>
                                    <td className="px-4 py-2.5">
                                        {!r.isActive || r.suppressed ? (
                                            <span className="inline-flex items-center gap-1 text-[11px] text-amber-600 dark:text-amber-400" title="Disabled/edge-off hosts are refused a certificate (the tls-ask endpoint returns 403) — TLS handshakes to them fail by design.">
                                                <AlertTriangle className="h-3 w-3" /> No cert · disabled
                                            </span>
                                        ) : r.tlsMode === "cf_origin" ? (
                                            <div className="flex flex-col gap-0.5">
                                                <span className="inline-flex w-fit items-center gap-1 rounded-full border border-sky-500/30 bg-sky-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-sky-600 dark:text-sky-400" title="Serves a static Cloudflare Origin certificate (proxied through Cloudflare)">
                                                    <ShieldCheck className="h-3 w-3" /> CF Origin
                                                </span>
                                                {(() => {
                                                    const sel = r.tlsCertPath ? undefined : originCertFor(r.host, originCerts);
                                                    const path = r.tlsCertPath || sel?.certPath;
                                                    return (
                                                        <span className="text-[10px] text-muted-foreground" title={path ? `${path}${sel ? ` (auto-selected — covers ${(sel.covers ?? []).join(", ")})` : " (pinned per-host)"}` : "no registered cert covers this host"}>
                                                            {path ? certBasename(path) : "⚠ no cert"}
                                                        </span>
                                                    );
                                                })()}
                                            </div>
                                        ) : (
                                            <span className="text-[11px] text-muted-foreground" title="On-demand Let's Encrypt (issued on first visit, authorized by the tls-ask endpoint)">On-demand</span>
                                        )}
                                    </td>
                                    <td className="px-4 py-2.5">
                                        <div className="flex justify-end gap-1">
                                            <button
                                                onClick={() => setTlsRow(r)}
                                                disabled={tlsBusy === r.host}
                                                className={`rounded p-1.5 ${r.tlsMode === "cf_origin" ? "text-sky-600 hover:bg-sky-500/10 dark:text-sky-400" : "text-muted-foreground hover:bg-accent hover:text-foreground"}`}
                                                title={r.tlsMode === "cf_origin" ? "Switch back to on-demand Let's Encrypt" : "Switch to Cloudflare Origin cert (for a CF-proxied host)"}
                                            >
                                                {tlsBusy === r.host ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Shield className="h-3.5 w-3.5" />}
                                            </button>
                                            <button
                                                onClick={() => toggleSuppress(r.host, !r.suppressed)}
                                                disabled={suppressBusy === r.host}
                                                className={`rounded p-1.5 ${r.suppressed ? "text-emerald-600 hover:bg-emerald-500/10 dark:text-emerald-400" : "text-muted-foreground hover:bg-accent hover:text-foreground"}`}
                                                title={r.suppressed ? "Enable serving at Caddy" : "Disable serving at Caddy (edge — DB row kept)"}
                                            >
                                                {suppressBusy === r.host ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : r.suppressed ? <Power className="h-3.5 w-3.5" /> : <PowerOff className="h-3.5 w-3.5" />}
                                            </button>
                                            <button onClick={() => setEdit(r)} className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground" title="Edit">
                                                <Pencil className="h-3.5 w-3.5" />
                                            </button>
                                            {r.isActive && !r.suppressed ? (
                                                <button
                                                    onClick={() => setPinRow(r)}
                                                    className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                                                    title="Pin — freeze this route as a static block in the main Caddyfile (off the reconcile path)"
                                                >
                                                    <Pin className="h-3.5 w-3.5" />
                                                </button>
                                            ) : null}
                                            <button onClick={() => del(r)} className="rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" title="Disable (soft-delete the row)">
                                                <Trash2 className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 ? (
                                <tr>
                                    <td colSpan={6} className="px-4 py-6 text-center text-muted-foreground">
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

            {removeBlock ? (
                <Modal onClose={() => (removing ? null : setRemoveBlock(null))} title={`Remove ${removeBlock.host} from the Caddyfile?`}>
                    <p className="text-xs text-muted-foreground">
                        This is server-panel's only edit to the main Caddyfile. It backs the file up, surgically removes <b>only</b> this host's
                        static block, re-validates with <code>caddy adapt</code>, asserts every other host + the dashboard/panel domains survive,
                        then reloads. If anything else would change, it <b>aborts and restores</b>. Gated by live reconcile.
                    </p>
                    <p className="mt-2 font-mono text-[11px] text-muted-foreground">
                        {removeBlock.host}
                        {removeBlock.upstreams && removeBlock.upstreams.length > 0 ? ` → ${removeBlock.upstreams.join(", ")}` : ""}
                    </p>
                    <div className="mt-5 flex justify-end gap-2">
                        <Button variant="outline" size="sm" onClick={() => setRemoveBlock(null)} disabled={removing}>
                            Cancel
                        </Button>
                        <Button variant="destructive" size="sm" className="gap-2" onClick={() => removePinnedBlock(removeBlock)} disabled={removing}>
                            {removing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                            Remove block
                        </Button>
                    </div>
                </Modal>
            ) : null}

            {pinRow ? (
                <Modal onClose={() => (converting ? null : setPinRow(null))} title={`Pin ${pinRow.host}?`}>
                    <p className="text-xs text-muted-foreground">
                        Pinning freezes this route as a hand-written <code>reverse_proxy</code> block in the main Caddyfile and removes its
                        database row — taking it <b>off the reconcile path</b> entirely. It backs the Caddyfile up, adds the block, re-validates
                        with <code>caddy adapt</code>, asserts the served host set is unchanged and the dashboard/panel survive, then reloads
                        (aborting + restoring on any mismatch). It lands as <b className="text-amber-600 dark:text-amber-400">Pinned · unmanaged</b>.
                        Gated by live reconcile.
                    </p>
                    <p className="mt-2 font-mono text-[11px] text-muted-foreground">
                        {pinRow.host} → {pinRow.target}
                    </p>
                    <div className="mt-5 flex justify-end gap-2">
                        <Button variant="outline" size="sm" onClick={() => setPinRow(null)} disabled={converting}>
                            Cancel
                        </Button>
                        <Button size="sm" className="gap-2" onClick={() => convert("pin", { id: pinRow.id }, pinRow.host, "pinned")} disabled={converting}>
                            {converting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Pin className="h-4 w-4" />}
                            Pin route
                        </Button>
                    </div>
                </Modal>
            ) : null}

            {unpinRow ? (
                <Modal onClose={() => (converting ? null : setUnpinRow(null))} title={`Unpin ${unpinRow.host}?`}>
                    <p className="text-xs text-muted-foreground">
                        Unpinning converts this static block back into a <b>managed system host</b>: it removes the block, renders a
                        reconcile-owned vhost file for the same upstream, re-validates with <code>caddy adapt</code>, asserts the served host
                        set is unchanged and the dashboard/panel survive, reloads (aborting + restoring on any mismatch), then adopts it as a
                        <code> platform_hosts</code> row. Gated by live reconcile.
                    </p>
                    <p className="mt-2 font-mono text-[11px] text-muted-foreground">
                        {unpinRow.host}
                        {unpinRow.upstreams && unpinRow.upstreams.length > 0 ? ` → ${unpinRow.upstreams.join(", ")}` : ""}
                    </p>
                    <div className="mt-5 flex justify-end gap-2">
                        <Button variant="outline" size="sm" onClick={() => setUnpinRow(null)} disabled={converting}>
                            Cancel
                        </Button>
                        <Button size="sm" className="gap-2" onClick={() => convert("unpin", { host: unpinRow.host }, unpinRow.host, "unpinned")} disabled={converting}>
                            {converting ? <Loader2 className="h-4 w-4 animate-spin" /> : <PinOff className="h-4 w-4" />}
                            Unpin route
                        </Button>
                    </div>
                </Modal>
            ) : null}

            {tlsRow ? (
                <TlsModeModal
                    row={tlsRow}
                    originCerts={originCerts}
                    busy={Boolean(tlsBusy)}
                    onApply={applyTlsMode}
                    onClose={() => setTlsRow(null)}
                />
            ) : null}

            {originOpen ? <OriginCertsModal certs={originCerts} onClose={() => setOriginOpen(false)} onChanged={onSaved} /> : null}
        </div>
    );
}

// certBasename shows just the filename of a cert path (the directory is in the title).
function certBasename(path: string): string {
    const parts = path.split("/");
    return parts[parts.length - 1] || path;
}

// TlsModeModal confirms switching a host's TLS mode. Switching TO cf_origin shows the
// certificate the panel AUTO-SELECTED for this hostname (matched against each
// registered cert's SANs) — no default, no typing. An optional per-host path override
// remains as an escape hatch for unusual cases.
function TlsModeModal({
    row,
    originCerts,
    busy,
    onApply,
    onClose,
}: {
    row: ManageRow;
    originCerts: OriginCert[];
    busy: boolean;
    onApply: (host: string, mode: string, certPath?: string, keyPath?: string) => void;
    onClose: () => void;
}) {
    const toCF = row.tlsMode !== "cf_origin";
    const [override, setOverride] = useState(false);
    const [cert, setCert] = useState(row.tlsCertPath ?? "");
    const [key, setKey] = useState(row.tlsKeyPath ?? "");
    const selected = originCertFor(row.host, originCerts);

    return (
        <Modal onClose={() => (busy ? null : onClose())} title={toCF ? `Switch ${row.host} to Cloudflare Origin cert?` : `Switch ${row.host} back to on-demand?`}>
            {toCF ? (
                <>
                    <p className="text-xs text-muted-foreground">
                        Renders <code>tls &lt;cert&gt; &lt;key&gt;</code> instead of <code>tls {"{"} on_demand {"}"}</code> — a static Cloudflare Origin
                        certificate. Use this <b>only for a host proxied through Cloudflare</b>: a proxied host can't complete an ACME challenge
                        (it terminates at the CF edge), so on-demand LE would fail. The two modes are mutually exclusive.
                    </p>
                    {selected ? (
                        <div className="mt-3 rounded-md border border-sky-500/25 bg-sky-500/[0.07] px-3 py-2">
                            <p className="text-xs text-foreground">
                                Will use <b className="font-mono">{certBasename(selected.certPath)}</b>
                            </p>
                            <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">
                                covers {(selected.covers ?? []).join(", ")}
                                {selected.expires ? ` · expires ${selected.expires}` : ""}
                            </p>
                            <p className="mt-0.5 break-all font-mono text-[10px] text-muted-foreground/70">{selected.certPath}</p>
                        </div>
                    ) : (
                        <div className="mt-3 flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                            <span>
                                No registered certificate covers <b>{row.host}</b>. Register this zone's Origin cert under <b>Origin certs</b> — or
                                name one explicitly below. Applying without either is refused.
                            </span>
                        </div>
                    )}
                    <button type="button" onClick={() => setOverride((v) => !v)} className="mt-2 text-[11px] font-medium text-primary hover:underline">
                        {override ? "Use the auto-selected certificate" : "Name a certificate explicitly (advanced)"}
                    </button>
                    {override ? (
                        <div className="mt-2 space-y-2">
                            <Field label="Certificate path" hint="Escape hatch for unusual cases — normally the panel picks the covering cert for you.">
                                <input value={cert} onChange={(e) => setCert(e.target.value)} placeholder="/etc/caddy/cf-origin/…pem" className={`${inputCls} font-mono`} />
                            </Field>
                            <Field label="Private key path">
                                <input value={key} onChange={(e) => setKey(e.target.value)} placeholder="/etc/caddy/cf-origin/…key" className={`${inputCls} font-mono`} />
                            </Field>
                        </div>
                    ) : null}
                    <p className="mt-2 text-[11px] text-muted-foreground">
                        The certificate must cover <b>{row.host}</b> — the panel verifies this and refuses a wrong-zone cert. Reconciles immediately
                        (validated adapt → reload); reversible.
                    </p>
                </>
            ) : (
                <p className="text-xs text-muted-foreground">
                    Returns <b>{row.host}</b> to <code>tls {"{"} on_demand {"}"}</code> (on-demand Let's Encrypt). Do this only after the host is
                    <b> no longer proxied through Cloudflare</b> — otherwise ACME will fail at the edge. Reconciles immediately.
                </p>
            )}
            <div className="mt-5 flex justify-end gap-2">
                <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>Cancel</Button>
                <Button
                    size="sm"
                    className="gap-2"
                    onClick={() => onApply(row.host, toCF ? "cf_origin" : "ondemand", override ? cert.trim() : "", override ? key.trim() : "")}
                    disabled={busy}
                >
                    {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Shield className="h-4 w-4" />}
                    {toCF ? "Use CF Origin cert" : "Use on-demand"}
                </Button>
            </div>
        </Modal>
    );
}

// OriginCertsModal manages the REGISTRY of Cloudflare Origin certificates. There is
// deliberately no "default" — propertyweb.co and propertyboom.co are peer zones, so
// the panel selects by hostname coverage instead of privileging one of them.
function OriginCertsModal({ certs, onClose, onChanged }: { certs: OriginCert[]; onClose: () => void; onChanged: () => void }) {
    const [c, setC] = useState("");
    const [k, setK] = useState("");
    const [saving, setSaving] = useState(false);

    const add = async () => {
        setSaving(true);
        try {
            const res = await fetch("/post/vhost/origin-cert", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ cert: c.trim(), key: k.trim() }),
            });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success("Certificate registered");
            setC("");
            setK("");
            onChanged();
        } catch (err) {
            toast.error(`Register failed: ${String(err)}`);
        } finally {
            setSaving(false);
        }
    };

    const remove = async (certPath: string) => {
        try {
            const res = await fetch(`/post/vhost/origin-cert?cert=${encodeURIComponent(certPath)}`, { method: "DELETE" });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success("Certificate unregistered");
            onChanged();
        } catch (err) {
            toast.error(`Remove failed: ${String(err)}`);
        }
    };

    return (
        <Modal onClose={onClose} title="Cloudflare Origin certificates">
            <p className="text-xs text-muted-foreground">
                Register one certificate per Cloudflare zone (Origin CA certs are per-zone). When a host is switched to <b>CF Origin</b>, the panel
                picks the registered certificate whose names cover that hostname — no default, nothing to type. Create them in the Cloudflare
                dashboard (SSL/TLS → Origin Server) and install the files on this host first.
            </p>

            <div className="mt-3 space-y-1.5">
                {certs.length === 0 ? (
                    <p className="rounded-md border border-dashed border-border px-3 py-4 text-center text-[11px] text-muted-foreground">
                        No certificates registered yet.
                    </p>
                ) : (
                    certs.map((entry) => (
                        <div key={entry.certPath} className="flex items-start justify-between gap-3 rounded-md border border-border bg-muted/30 px-3 py-2">
                            <div className="min-w-0">
                                <p className="font-mono text-[11px] font-medium text-foreground">{certBasename(entry.certPath)}</p>
                                {entry.error ? (
                                    <p className="mt-0.5 flex items-start gap-1 text-[11px] text-destructive">
                                        <AlertTriangle className="mt-px h-3 w-3 shrink-0" /> {entry.error}
                                    </p>
                                ) : (
                                    <p className="mt-0.5 text-[11px] text-muted-foreground">
                                        covers {(entry.covers ?? []).join(", ")}
                                        {entry.expires ? ` · expires ${entry.expires}` : ""}
                                    </p>
                                )}
                                <p className="mt-0.5 break-all font-mono text-[10px] text-muted-foreground/60">{entry.certPath}</p>
                            </div>
                            <button onClick={() => remove(entry.certPath)} className="shrink-0 rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" title="Unregister">
                                <Trash2 className="h-3.5 w-3.5" />
                            </button>
                        </div>
                    ))
                )}
            </div>

            <div className="mt-4 space-y-2 border-t border-border pt-3">
                <span className="text-xs font-medium text-foreground">Register a certificate</span>
                <Field label="Certificate path">
                    <input value={c} onChange={(e) => setC(e.target.value)} placeholder="/etc/caddy/cf-origin/propertyboom.pem" className={`${inputCls} font-mono`} />
                </Field>
                <Field label="Private key path">
                    <input value={k} onChange={(e) => setK(e.target.value)} placeholder="/etc/caddy/cf-origin/propertyboom.key" className={`${inputCls} font-mono`} />
                </Field>
                <div className="flex justify-end gap-2 pt-1">
                    <Button variant="outline" size="sm" onClick={onClose}>Close</Button>
                    <Button size="sm" className="gap-2" onClick={add} disabled={saving || !c.trim() || !k.trim()}>
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                        Register
                    </Button>
                </div>
            </div>
        </Modal>
    );
}

// vhostFileName mirrors render.FileName: "<host>.caddy", with "*.x" → "wildcard_x.caddy".
function vhostFileName(host: string): string {
    const h = host.trim().toLowerCase();
    if (!h) return "<host>.caddy";
    return h.startsWith("*.") ? `wildcard_${h.slice(2)}.caddy` : `${h}.caddy`;
}

// renderedVhost mirrors the engine's system-host render (render.HeaderDirectives +
// proxySnippet): configured response headers (sorted, quoted) before reverse_proxy.
// Global encode/security-header policy, if enabled, is applied at render and not shown.
function renderedVhost(host: string, target: string, headers: { name: string; value: string }[]): string {
    const lines = headers
        .filter((h) => h.name.trim())
        .map((h) => `    header ${h.name.trim()} "${h.value}"`)
        .sort();
    lines.push(`    reverse_proxy ${target.trim() || "<backend>"}`);
    return `${host.trim() || "<host>"} {\n${lines.join("\n")}\n}\n`;
}

function HostForm({ row, upstreams, onClose, onSaved }: { row: ManageRow; upstreams: Upstream[]; onClose: () => void; onSaved: () => void }) {
    const [host, setHost] = useState(row.host);
    const [target, setTarget] = useState(row.target);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);
    const [showSuggest, setShowSuggest] = useState(false);
    const [headerRows, setHeaderRows] = useState<{ name: string; value: string }[]>(
        Object.entries(row.headers ?? {}).map(([name, value]) => ({ name, value })),
    );

    const updateHeader = (i: number, key: "name" | "value", val: string) =>
        setHeaderRows((rows) => rows.map((r, j) => (j === i ? { ...r, [key]: val } : r)));
    const removeHeader = (i: number) => setHeaderRows((rows) => rows.filter((_, j) => j !== i));

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
            const headers: Record<string, string> = {};
            for (const h of headerRows) {
                const n = h.name.trim();
                if (n) headers[n] = h.value;
            }
            const res = await fetch("/post/vhost/system", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), serverStack, target: t, isActive, headers }),
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
                <div>
                    <span className="mb-1 block text-xs font-medium text-foreground">Response headers</span>
                    <p className="mb-2 text-[11px] text-muted-foreground">
                        Set on every response, including reverse_proxy 404/502. e.g. <code>X-Robots-Tag: noindex</code>. Names: letters, digits, hyphens.
                    </p>
                    <div className="space-y-1.5">
                        {headerRows.map((h, i) => (
                            <div key={i} className="flex items-center gap-1.5">
                                <input value={h.name} onChange={(e) => updateHeader(i, "name", e.target.value)} placeholder="X-Robots-Tag" className={`${inputCls} flex-1 font-mono`} autoComplete="off" />
                                <span className="text-muted-foreground">:</span>
                                <input value={h.value} onChange={(e) => updateHeader(i, "value", e.target.value)} placeholder="noindex" className={`${inputCls} flex-1 font-mono`} autoComplete="off" />
                                <button type="button" onClick={() => removeHeader(i)} className="rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" title="Remove header">
                                    <Trash2 className="h-3.5 w-3.5" />
                                </button>
                            </div>
                        ))}
                    </div>
                    <button type="button" onClick={() => setHeaderRows([...headerRows, { name: "", value: "" }])} className="mt-1.5 inline-flex items-center gap-1 text-[11px] font-medium text-primary hover:underline">
                        <Plus className="h-3 w-3" /> Add header
                    </button>
                </div>
                <div>
                    <div className="mb-1 flex items-center justify-between">
                        <span className="text-xs font-medium text-foreground">Rendered vhost</span>
                        <code className="text-[11px] text-muted-foreground">{vhostFileName(host)}</code>
                    </div>
                    <pre className="overflow-x-auto rounded-md border border-border bg-zinc-950 p-3 font-mono text-[11px] leading-5 text-zinc-200">{renderedVhost(host, target, headerRows)}</pre>
                    <p className="mt-1 text-[11px] text-muted-foreground">
                        Generated from the fields above — the reconcile engine writes exactly this. Read-only (the DB row is the source of truth; editing the file directly would be overwritten).
                    </p>
                </div>
            </div>
            <FormActions saving={saving} onCancel={onClose} onSave={save} disabled={!host.trim() || !target.trim()} />
        </Modal>
    );
}
