import { useState } from "react";
import { AlertTriangle, CheckCircle2, Database, Lock, RefreshCw, ShieldCheck, Zap } from "lucide-react";

import { Button } from "_layouts/_components/ui/button";
import { Modal, Pill, type ReconcileResult, type Source, type VhostState } from "./shared";

export default function ReconcileHeader({
    state,
    sources,
    savingSource,
    applying,
    result,
    onChangeSource,
    onApply,
    onDismissResult,
}: {
    state: VhostState;
    sources: Source[];
    savingSource: boolean;
    applying: "reconcile" | "reload" | null;
    result: ReconcileResult | null;
    onChangeSource: (name: string) => void;
    onApply: (kind: "reconcile" | "reload") => void;
    onDismissResult: () => void;
}) {
    const [confirm, setConfirm] = useState<null | "reconcile" | "reload">(null);
    const dry = state.dryRun;
    const live = state.liveReload;
    const hostsOnDisk = dry?.files.length ?? 0;
    const writes = dry?.would_write ?? [];
    const removes = dry?.would_remove ?? [];
    const pending = writes.length + removes.length;
    const orphanCount = dry?.orphans.length ?? 0;
    const configured = state.configured && !state.error;

    return (
        <div className="border-b border-border bg-card/40">
            <div className="flex flex-wrap items-center gap-x-6 gap-y-3 px-6 py-4">
                <Stat n={hostsOnDisk} label="Hosts on disk" />
                <span className="hidden h-8 w-px bg-border sm:block" />
                <Stat n={pending} label="Pending" tone={pending > 0 ? "warn" : undefined} />
                <span className="hidden h-8 w-px bg-border sm:block" />
                <Stat n={orphanCount} label="Orphans" tone={orphanCount > 0 ? "err" : undefined} />
                <span className="flex-1" />
                {dry ? dry.in_sync ? <Pill tone="ok">In sync</Pill> : <Pill tone="warn">Drift — {pending} pending</Pill> : null}
                <label className="inline-flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
                    <Database className="h-3.5 w-3.5" />
                    Reading from
                    <select
                        value={state.source ?? ""}
                        onChange={(e) => onChangeSource(e.target.value)}
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
                <div className="flex items-center gap-2">
                    <Button variant="outline" size="sm" className="gap-2" disabled={!configured || applying !== null} onClick={() => setConfirm("reload")}>
                        <RefreshCw className={`h-4 w-4 ${applying === "reload" ? "animate-spin" : ""}`} />
                        Force reload
                    </Button>
                    <Button size="sm" className="gap-2" disabled={!configured || applying !== null} onClick={() => setConfirm("reconcile")}>
                        <Zap className="h-4 w-4" />
                        {pending > 0 ? `Reconcile all (${pending})` : "Reconcile all"}
                    </Button>
                </div>
            </div>

            {/* live-reload gate banner + optional preview */}
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-6 pb-3">
                <span
                    className={`inline-flex items-center gap-1.5 text-[11px] font-medium ${
                        live ? "text-emerald-600 dark:text-emerald-400" : "text-amber-600 dark:text-amber-400"
                    }`}
                >
                    {live ? <ShieldCheck className="h-3.5 w-3.5" /> : <Lock className="h-3.5 w-3.5" />}
                    {live ? "Live reconcile armed — validated adapt → reload" : "Live reconcile gated (CADDY_LIVE_RELOAD off) — edits save; no reload until an operator arms it"}
                </span>
                {pending > 0 ? (
                    <span className="text-[11px] text-muted-foreground">
                        preview: <b className="text-amber-600 dark:text-amber-400">{writes.length}</b> to write ·{" "}
                        <b className="text-destructive">{removes.length}</b> to remove
                    </span>
                ) : null}
            </div>

            {result ? (
                <div className="px-6 pb-4">
                    <ResultPanel result={result} onDismiss={onDismissResult} />
                </div>
            ) : null}

            {confirm ? (
                <ConfirmApply
                    kind={confirm}
                    live={live}
                    writes={writes}
                    removes={removes}
                    onCancel={() => setConfirm(null)}
                    onConfirm={() => {
                        const k = confirm;
                        setConfirm(null);
                        onApply(k);
                    }}
                />
            ) : null}
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

function ConfirmApply({
    kind,
    live,
    writes,
    removes,
    onCancel,
    onConfirm,
}: {
    kind: "reconcile" | "reload";
    live: boolean;
    writes: string[];
    removes: string[];
    onCancel: () => void;
    onConfirm: () => void;
}) {
    return (
        <Modal onClose={onCancel} title={kind === "reload" ? "Re-validate & reload Caddy" : "Reconcile all & reload Caddy"}>
            {!live ? (
                <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    <Lock className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>
                        Live reconcile is <b>not enabled</b>. This returns the gate message without touching Caddy — safe to run to verify the gate.
                    </span>
                </div>
            ) : null}
            <p className="text-xs text-muted-foreground">
                {kind === "reload"
                    ? "Re-adapts the current folder and reloads Caddy — no files are written or removed."
                    : "Renders all three tables, validates with caddy adapt, asserts the dashboard survives, backs up the prior config, then reloads via the admin API. The drop-guard refuses if any live host would vanish."}
            </p>
            {kind === "reconcile" ? (
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    <DiffList tone="warn" label="Will write" items={writes} />
                    <DiffList tone="err" label="Will remove" items={removes} />
                </div>
            ) : null}
            <div className="mt-5 flex justify-end gap-2">
                <Button variant="outline" size="sm" onClick={onCancel}>
                    Cancel
                </Button>
                <Button size="sm" className="gap-2" onClick={onConfirm}>
                    <Zap className="h-4 w-4" />
                    {kind === "reload" ? "Reload now" : "Apply & reload"}
                </Button>
            </div>
        </Modal>
    );
}

function DiffList({ tone, label, items }: { tone: "warn" | "err"; label: string; items: string[] }) {
    const color = tone === "warn" ? "text-amber-600 dark:text-amber-400" : "text-destructive";
    return (
        <div className="rounded-md border border-border bg-muted/30 p-3">
            <p className={`text-[11px] font-semibold uppercase tracking-wide ${color}`}>
                {label} ({items.length})
            </p>
            {items.length === 0 ? (
                <p className="mt-1 text-[11px] text-muted-foreground">None</p>
            ) : (
                <ul className="mt-1 max-h-32 space-y-0.5 overflow-y-auto">
                    {items.map((i) => (
                        <li key={i} className="truncate font-mono text-[11px] text-foreground">
                            {i}
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

function ResultPanel({ result, onDismiss }: { result: ReconcileResult; onDismiss: () => void }) {
    const ok = !result.error;
    return (
        <div className={`rounded-md border p-4 ${ok ? "border-emerald-500/25 bg-emerald-500/[0.05]" : "border-destructive/30 bg-destructive/[0.06]"}`}>
            <div className="flex items-start gap-2">
                {ok ? (
                    <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                ) : (
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                )}
                <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-foreground">
                        {result.reloaded ? "Caddy reloaded" : ok ? "Reconcile complete — no reload" : "Not applied"}
                        <span className="ml-2 text-[11px] font-normal text-muted-foreground">{result.duration_ms}ms</span>
                    </p>
                    <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
                        <span>
                            reloaded: <b className="text-foreground">{String(result.reloaded)}</b>
                        </span>
                        <span>
                            written: <b className="text-foreground">{result.written.length}</b>
                        </span>
                        <span>
                            removed: <b className="text-foreground">{result.removed.length}</b>
                        </span>
                        {result.removes_suppressed && result.removes_suppressed.length > 0 ? (
                            <span>
                                suppressed (first pass): <b className="text-foreground">{result.removes_suppressed.length}</b>
                            </span>
                        ) : null}
                        <span>
                            orphans: <b className="text-foreground">{result.orphans.length}</b>
                        </span>
                    </div>
                    {result.blocked_drops && result.blocked_drops.length > 0 ? (
                        <div className="mt-2 rounded-md border border-destructive/30 bg-destructive/10 p-2.5">
                            <p className="text-[11px] font-semibold text-destructive">
                                Outage guard: refused — {result.blocked_drops.length} live host(s) would have been dropped
                            </p>
                            <ul className="mt-1 max-h-32 space-y-0.5 overflow-y-auto">
                                {result.blocked_drops.map((h) => (
                                    <li key={h} className="font-mono text-[11px] text-foreground">
                                        {h}
                                    </li>
                                ))}
                            </ul>
                            <p className="mt-1 text-[11px] text-muted-foreground">
                                Caddy was not touched — these are served now but missing from the new config. Fix the source/import before reloading.
                            </p>
                        </div>
                    ) : null}
                    {result.error ? <p className="mt-2 font-mono text-[11px] text-destructive">{result.error}</p> : null}
                    {result.adapt_warnings && result.adapt_warnings.length > 0 ? (
                        <details className="mt-2">
                            <summary className="cursor-pointer text-[11px] text-amber-600 dark:text-amber-400">
                                {result.adapt_warnings.length} adapt warning(s)
                            </summary>
                            <ul className="mt-1 space-y-0.5">
                                {result.adapt_warnings.map((w, i) => (
                                    <li key={i} className="font-mono text-[11px] text-muted-foreground">
                                        {w}
                                    </li>
                                ))}
                            </ul>
                        </details>
                    ) : null}
                    {result.backup_path ? <p className="mt-1 font-mono text-[11px] text-muted-foreground">backup: {result.backup_path}</p> : null}
                </div>
                <button onClick={onDismiss} className="text-[11px] text-muted-foreground hover:text-foreground">
                    Dismiss
                </button>
            </div>
        </div>
    );
}
