import { useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, Copy, Loader2, Lock, Mail, ShieldAlert, X } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import { arr, inputCls } from "./shared";
import { type CutoverVars, methodAMessage, methodBMessage, methodCMessage, wwwWarning } from "./cutover-templates";

// Cutover Assistant — per-tenant client message generator.
// Spec: design-templates docs/panel-cutover-assistant.md @ 1a5eb56.
//
// READ + COMPOSE + DISPLAY ONLY. website_hosts is stack-owned: this modal performs
// DNS lookups and assembles text. It never writes a row, never changes a host's
// enabled state, and never touches Caddy config.

type DNSRecordRow = { label: string; type: string; group: string; values: string[] };

type CutoverInfo = {
    host: string;
    edge: string;
    oldIp?: string;
    oldIps?: string[];
    hasMx: boolean;
    mx?: string[];
    hasWww: boolean;
    www?: string[];
    records: DNSRecordRow[];
    nameservers?: string[];
    dsPresent: boolean;
    dsChecked: boolean;
    dsValues?: string[];
    dsNote?: string;
};

type DiffRow = { label: string; type: string; current: string[]; target: string[]; match: boolean };
type DiffResult = { rows: DiffRow[]; allMatch: boolean; mismatch: number; error?: string };

async function copyText(text: string, what: string) {
    try {
        await navigator.clipboard.writeText(text);
        toast.success(`${what} copied`);
    } catch {
        toast.error("Could not copy — select the text and copy manually");
    }
}

function CopyButton({ text, label = "Copy", what = "Message" }: { text: string; label?: string; what?: string }) {
    return (
        <button
            type="button"
            onClick={() => copyText(text, what)}
            className="inline-flex items-center gap-1 rounded border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground hover:bg-muted hover:text-foreground"
        >
            <Copy className="h-3 w-3" /> {label}
        </button>
    );
}

// MessagePane renders an assembled message with its primary copy button. Plain
// <pre> so what's copied is exactly what's shown — no markdown artefacts.
function MessagePane({ text, locked, lockReason }: { text: string; locked?: boolean; lockReason?: string }) {
    if (locked) {
        return (
            <div className="rounded-md border border-dashed border-border bg-muted/20 p-6 text-center">
                <Lock className="mx-auto mb-2 h-5 w-5 text-muted-foreground" />
                <p className="text-xs font-medium text-foreground">Message locked</p>
                <p className="mx-auto mt-1 max-w-md text-[11px] text-muted-foreground">{lockReason}</p>
            </div>
        );
    }
    return (
        <div>
            <div className="mb-2 flex justify-end">
                <Button size="sm" className="gap-2" onClick={() => copyText(text, "Message")}>
                    <Copy className="h-4 w-4" /> Copy message
                </Button>
            </div>
            <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-border bg-zinc-950 p-4 font-mono text-[11px] leading-5 text-zinc-200">{text}</pre>
        </div>
    );
}

export default function CutoverModal({ host, edge, onClose }: { host: string; edge?: string; onClose: () => void }) {
    const [tab, setTab] = useState<"A" | "C" | "B">("A");
    const [clientName, setClientName] = useState("");
    const [info, setInfo] = useState<CutoverInfo | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");

    // Phase 1: Cloudflare validation records + nameservers are pasted from the
    // dashboard. Phase 2 (TODO) reads them back from the Cloudflare API.
    const [txt, setTxt] = useState([
        { name: "_cf-custom-hostname", value: "" },
        { name: "", value: "" },
        { name: "", value: "" },
    ]);
    const [nsInput, setNsInput] = useState("");

    const [diff, setDiff] = useState<DiffResult | null>(null);
    const [diffBusy, setDiffBusy] = useState(false);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const res = await fetch(`/post/vhost/cutover?host=${encodeURIComponent(host)}`, { cache: "no-store" });
                if (!res.ok) throw new Error((await res.text()).trim() || "Pre-flight scan failed");
                const data = (await res.json()) as CutoverInfo;
                // Go marshals a nil slice as JSON null, and most probed labels have
                // no record — coerce every list so the render can iterate freely.
                data.records = arr(data.records).map((r) => ({ ...r, values: arr(r.values) }));
                if (!cancelled) setInfo(data);
            } catch (err) {
                if (!cancelled) setError(err instanceof Error ? err.message : "Pre-flight scan failed");
            } finally {
                if (!cancelled) setLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [host]);

    const nameservers = nsInput
        .split(/[\s,]+/)
        .map((n) => n.trim())
        .filter(Boolean);

    const vars: CutoverVars = {
        domain: host,
        name: clientName,
        oldIp: info?.oldIp ?? "",
        txt,
        nameservers,
    };
    const hasMx = Boolean(info?.hasMx);

    // ---- Tab A gates ----
    // DNSSEC: a DS record makes a nameserver switch break the domain outright.
    // UNKNOWN is treated as NOT SAFE — the gate stays closed rather than guessing.
    const dnssecGreen = Boolean(info && info.dsChecked && !info.dsPresent);
    const diffGreen = Boolean(diff && !diff.error && diff.allMatch && diff.rows.length > 0);
    const stage4Locked = !dnssecGreen || !diffGreen;
    const lockReason = !info
        ? "Waiting for the pre-flight scan."
        : !info.dsChecked
          ? `DNSSEC could not be verified. ${info.dsNote ?? ""} The gate stays closed until it is confirmed absent.`
          : info.dsPresent
            ? "DNSSEC is enabled (a DS record exists). Changing nameservers now makes the domain UNRESOLVABLE. The client must disable DNSSEC at the registrar first, wait for it to clear, then proceed."
            : !diff
              ? "Run the zone diff (Stage 3): every record must answer identically from the current and the Cloudflare nameservers before a switch is safe."
              : diff.error
                ? `Zone diff could not run: ${diff.error}`
                : `Zone diff has ${diff.mismatch} mismatching record(s). A mismatch means a record was dropped or altered on import — switching nameservers would make that loss live.`;

    const runDiff = async () => {
        setDiffBusy(true);
        try {
            const res = await fetch("/post/vhost/cutover/diff", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ host, nameservers }),
            });
            const data = (await res.json()) as DiffResult;
            data.rows = arr(data.rows).map((r) => ({ ...r, current: arr(r.current), target: arr(r.target) }));
            setDiff(data);
        } catch (err) {
            setDiff({ rows: [], allMatch: false, mismatch: 0, error: String(err) });
        } finally {
            setDiffBusy(false);
        }
    };

    const groups = Array.from(new Set((info?.records ?? []).map((r) => r.group)));

    return (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={onClose}>
            <div className="flex h-[min(880px,92vh)] w-full max-w-4xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl" onClick={(e) => e.stopPropagation()}>
                {/* Header */}
                <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
                    <div className="min-w-0">
                        <h2 className="truncate text-sm font-semibold text-foreground">Cutover · {host}</h2>
                        <p className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                            <span>
                                Edge: <b className="text-foreground">{info?.edge || edge || "—"}</b>
                            </span>
                            {info?.oldIp ? (
                                <>
                                    <span className="text-muted-foreground/40">·</span>
                                    <span>
                                        current A: <b className="font-mono text-foreground">{info.oldIp}</b>
                                    </span>
                                </>
                            ) : null}
                            <span className="text-muted-foreground/40">·</span>
                            <span>read-only — this never modifies the host</span>
                        </p>
                    </div>
                    <Button size="icon" variant="ghost" className="h-8 w-8 shrink-0" onClick={onClose} aria-label="Close">
                        <X className="h-4 w-4" />
                    </Button>
                </div>

                {/* Client name + warnings */}
                <div className="space-y-2 border-b border-border px-5 py-3">
                    <label className="block">
                        <span className="mb-1 block text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Client name</span>
                        <input value={clientName} onChange={(e) => setClientName(e.target.value)} placeholder="e.g. Raymond" className={inputCls} autoFocus />
                    </label>
                    {info?.hasMx ? (
                        <div className="flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
                            <Mail className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                            <span>
                                This domain carries email ({(info.mx ?? []).join(", ")}). The "your email is unaffected" paragraph is appended to every message
                                automatically — a client who fears for their email stalls for weeks.
                            </span>
                        </div>
                    ) : null}
                    {info?.hasWww ? (
                        <div className="flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
                            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                            <span>{wwwWarning(host)}</span>
                        </div>
                    ) : null}
                </div>

                {/* Tabs — ranked A > C > B */}
                <div className="flex items-center gap-1 border-b border-border px-5 pt-3">
                    {([
                        ["A", "A · Nameservers", "first priority"],
                        ["C", "C · TXT validation", "fallback"],
                        ["B", "B · HTTP validation", "superseded"],
                    ] as const).map(([key, label, hint]) => (
                        <button
                            key={key}
                            onClick={() => setTab(key)}
                            className={`rounded-t-md border-b-2 px-3 py-2 text-xs font-medium ${
                                tab === key ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground"
                            } ${key === "B" ? "opacity-60" : ""}`}
                        >
                            {label}
                            <span className="ml-1.5 text-[10px] text-muted-foreground">{hint}</span>
                        </button>
                    ))}
                </div>

                <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
                    {loading ? (
                        <div className="flex h-40 items-center justify-center gap-2 text-xs text-muted-foreground">
                            <Loader2 className="h-5 w-5 animate-spin" /> Running the pre-flight scan…
                        </div>
                    ) : error ? (
                        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-xs text-destructive">{error}</div>
                    ) : tab === "A" ? (
                        <div className="space-y-5">
                            {/* Stage 1 */}
                            <section>
                                <h3 className="text-xs font-semibold text-foreground">Stage 1 · Pre-flight scan</h3>
                                <p className="mt-0.5 text-[11px] text-muted-foreground">
                                    DNS can't be enumerated (zone transfer is refused), so this probes a fixed label list and reports everything that answers.
                                    Read it to the client and ask: <i>"do you use this domain for anything besides the website and email?"</i> — that catches
                                    what the probe can't.
                                </p>
                                <div className="mt-2 space-y-3">
                                    {groups.map((g) => {
                                        const rows = (info?.records ?? []).filter((r) => r.group === g);
                                        const found = rows.filter((r) => r.values.length > 0);
                                        return (
                                            <div key={g} className="rounded-md border border-border">
                                                <div className="flex items-center justify-between border-b border-border bg-muted/30 px-3 py-1.5">
                                                    <span className="text-[11px] font-semibold text-foreground">{g}</span>
                                                    <span className="text-[10px] text-muted-foreground">
                                                        {found.length} of {rows.length} answered
                                                    </span>
                                                </div>
                                                <div className="divide-y divide-border">
                                                    {rows.map((r) => (
                                                        <div key={`${r.label}-${r.type}`} className={`flex items-start gap-3 px-3 py-1.5 ${r.values.length ? "" : "opacity-45"}`}>
                                                            <span className="w-44 shrink-0 font-mono text-[11px] text-foreground">
                                                                {r.label} <span className="text-muted-foreground">{r.type}</span>
                                                            </span>
                                                            <div className="min-w-0 flex-1">
                                                                {r.values.length ? (
                                                                    r.values.map((v, i) => (
                                                                        <div key={i} className="flex items-start gap-1.5">
                                                                            <span className="min-w-0 flex-1 break-all font-mono text-[11px] text-muted-foreground">{v}</span>
                                                                            <CopyButton text={v} what="Record" />
                                                                        </div>
                                                                    ))
                                                                ) : (
                                                                    <span className="text-[11px] text-muted-foreground">—</span>
                                                                )}
                                                            </div>
                                                        </div>
                                                    ))}
                                                </div>
                                            </div>
                                        );
                                    })}
                                </div>
                            </section>

                            {/* Stage 2 — DNSSEC gate */}
                            <section>
                                <h3 className="text-xs font-semibold text-foreground">Stage 2 · DNSSEC gate</h3>
                                {dnssecGreen ? (
                                    <div className="mt-1 flex items-start gap-2 rounded-md border border-emerald-500/25 bg-emerald-500/10 px-3 py-2 text-[11px] text-emerald-700 dark:text-emerald-300">
                                        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                        <span>No DS record — DNSSEC is not enabled, so a nameserver change is safe on this count.</span>
                                    </div>
                                ) : (
                                    <div className="mt-1 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-[11px] text-destructive">
                                        <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                        <span>
                                            {info?.dsPresent ? (
                                                <>
                                                    <b>DNSSEC is enabled</b> (DS record present). Switching nameservers now makes the domain{" "}
                                                    <b>unresolvable</b> — not degraded. Remedy: the client disables DNSSEC at the registrar, waits for it to
                                                    clear, then you re-run this scan.
                                                </>
                                            ) : (
                                                <>
                                                    <b>DNSSEC could not be verified.</b> {info?.dsNote} Treated as not safe — the gate stays closed rather
                                                    than guessing.
                                                </>
                                            )}
                                        </span>
                                    </div>
                                )}
                            </section>

                            {/* Stage 3 — zone diff gate */}
                            <section>
                                <h3 className="text-xs font-semibold text-foreground">Stage 3 · Zone diff gate</h3>
                                <p className="mt-0.5 text-[11px] text-muted-foreground">
                                    Import the zone into Cloudflare first (DNS-only, nothing proxied). Then enter Cloudflare's assigned nameservers and diff:
                                    every label must answer identically from both sets. A mismatch means a record was dropped on import.
                                </p>
                                <div className="mt-2 flex flex-wrap items-end gap-2">
                                    <label className="min-w-[16rem] flex-1">
                                        <span className="mb-1 block text-[10px] uppercase tracking-wide text-muted-foreground">Cloudflare nameservers</span>
                                        <input value={nsInput} onChange={(e) => setNsInput(e.target.value)} placeholder="alice.ns.cloudflare.com  bob.ns.cloudflare.com" className={`${inputCls} font-mono`} />
                                    </label>
                                    <Button size="sm" variant="outline" onClick={runDiff} disabled={diffBusy || nameservers.length === 0}>
                                        {diffBusy ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
                                        Run diff
                                    </Button>
                                </div>
                                {info?.nameservers?.length ? (
                                    <p className="mt-1 font-mono text-[10px] text-muted-foreground">current: {info.nameservers.join(", ")}</p>
                                ) : null}
                                {diff ? (
                                    diff.error ? (
                                        <div className="mt-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">{diff.error}</div>
                                    ) : (
                                        <div className="mt-2 overflow-hidden rounded-md border border-border">
                                            <div className={`px-3 py-1.5 text-[11px] font-semibold ${diff.allMatch ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : "bg-destructive/10 text-destructive"}`}>
                                                {diff.allMatch ? "All records match — safe to switch nameservers" : `${diff.mismatch} mismatching record(s) — do not switch`}
                                            </div>
                                            <div className="max-h-56 divide-y divide-border overflow-y-auto">
                                                {diff.rows.filter((r) => r.current.length || r.target.length).map((r) => (
                                                    <div key={`${r.label}-${r.type}`} className="grid grid-cols-[10rem_1fr_1fr] gap-2 px-3 py-1.5 text-[10px]">
                                                        <span className="font-mono text-foreground">
                                                            {r.label} <span className="text-muted-foreground">{r.type}</span>
                                                        </span>
                                                        <span className="break-all font-mono text-muted-foreground">{r.current.join(", ") || "—"}</span>
                                                        <span className={`break-all font-mono ${r.match ? "text-muted-foreground" : "text-destructive"}`}>{r.target.join(", ") || "—"}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        </div>
                                    )
                                ) : null}
                            </section>

                            {/* Stage 4 — the message */}
                            <section>
                                <h3 className="text-xs font-semibold text-foreground">Stage 4 · Nameserver message</h3>
                                <div className="mt-2">
                                    <MessagePane text={methodAMessage(vars, hasMx)} locked={stage4Locked} lockReason={lockReason} />
                                </div>
                            </section>
                        </div>
                    ) : tab === "C" ? (
                        <div className="space-y-4">
                            <p className="text-[11px] text-muted-foreground">
                                Create the custom hostname in Cloudflare with <b>TXT Validation</b>, then paste the records it displays below — verbatim,
                                don't retype. The ten-minute pause in the message is what prevents the certificate race; don't edit it out.
                            </p>
                            <div className="space-y-2">
                                {txt.map((t, i) => (
                                    <div key={i} className="flex flex-wrap items-center gap-2">
                                        <span className="w-14 text-[10px] uppercase tracking-wide text-muted-foreground">TXT {i + 1}</span>
                                        <input
                                            value={t.name}
                                            onChange={(e) => setTxt((prev) => prev.map((p, j) => (j === i ? { ...p, name: e.target.value } : p)))}
                                            placeholder="_cf-custom-hostname"
                                            className={`${inputCls} w-56 font-mono`}
                                        />
                                        <input
                                            value={t.value}
                                            onChange={(e) => setTxt((prev) => prev.map((p, j) => (j === i ? { ...p, value: e.target.value } : p)))}
                                            placeholder="value from Cloudflare"
                                            className={`${inputCls} min-w-[12rem] flex-1 font-mono`}
                                        />
                                        <CopyButton text={t.value} what="Value" />
                                    </div>
                                ))}
                                <p className="text-[10px] text-muted-foreground">
                                    Name is the label only — <code>_cf-custom-hostname</code>, not <code>_cf-custom-hostname.{host}</code>. Registrars append
                                    the domain; the full form silently never validates.
                                </p>
                            </div>
                            <MessagePane text={methodCMessage(vars, hasMx)} />
                        </div>
                    ) : (
                        <div className="space-y-4">
                            <div className="flex items-start gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground">
                                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                <span>
                                    <b>Superseded — ranked last.</b> The client still adds <code>_cf-custom-hostname</code> <i>and</i> still changes the A
                                    record, so it saves them nothing while adding an SSH step for us. Kept only to finish a domain already started on this
                                    path.
                                </span>
                            </div>
                            <div className="flex flex-wrap items-center gap-2">
                                <span className="w-14 text-[10px] uppercase tracking-wide text-muted-foreground">TXT 1</span>
                                <input
                                    value={txt[0].name}
                                    onChange={(e) => setTxt((prev) => prev.map((p, j) => (j === 0 ? { ...p, name: e.target.value } : p)))}
                                    className={`${inputCls} w-56 font-mono`}
                                />
                                <input
                                    value={txt[0].value}
                                    onChange={(e) => setTxt((prev) => prev.map((p, j) => (j === 0 ? { ...p, value: e.target.value } : p)))}
                                    placeholder="value from Cloudflare"
                                    className={`${inputCls} min-w-[12rem] flex-1 font-mono`}
                                />
                            </div>
                            <MessagePane text={methodBMessage(vars, hasMx)} />
                        </div>
                    )}
                </div>

                <div className="border-t border-border px-5 py-2 text-[10px] text-muted-foreground">
                    Phase 1 · validation records are pasted from the Cloudflare dashboard. Phase 2 (TODO, needs a scoped API token) creates the custom
                    hostname and reads these back automatically.
                </div>
            </div>
        </div>
    );
}
