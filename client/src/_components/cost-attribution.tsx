import { useEffect, useRef, useState } from "react";
import { DollarSign, Info, Pencil } from "lucide-react";

import { type ContainerStat, fmtBytes } from "_components/container-stats";

// CostModel is the operator-supplied pricing so the panel can turn usage into $.
// Persisted in localStorage — it's per-operator config, not server state.
type CostModel = {
    computeMonthly: number; // instance $/mo (fixed — you rent the whole box)
    storageMonthly: number; // EBS $/mo (fixed — provisioned GB-month)
    egressGBMonthly: number; // billable GB out per month (from CloudWatch / bill, net of free tier)
    egressPerGB: number; // $/GB egress
};

const STORAGE_KEY = "cost_model";
const EMPTY: CostModel = { computeMonthly: 0, storageMonthly: 0, egressGBMonthly: 0, egressPerGB: 0.09 };

function loadModel(): CostModel {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) return { ...EMPTY, ...JSON.parse(raw) };
    } catch {
        /* ignore */
    }
    return EMPTY;
}

// CostAttributionPanel: fixed vs variable bill split, plus a rough per-container
// egress attribution from live tx-rate SHARE (deltas between polls, so a container
// restart resetting its counter doesn't skew it). Data comes from the shared stats
// hook via props — no extra polling.
export default function CostAttributionPanel({ stats }: { stats: ContainerStat[] | null }) {
    const [model, setModel] = useState<CostModel>(loadModel);
    const [editing, setEditing] = useState(false);

    // Smoothed per-container tx rate (bytes/s) from tx-counter deltas between polls.
    const prevRef = useRef<{ tx: Record<string, number>; t: number } | null>(null);
    const [rates, setRates] = useState<Record<string, number>>({});

    useEffect(() => {
        if (!stats) return;
        const now = Date.now();
        const prev = prevRef.current;
        const tx: Record<string, number> = {};
        for (const s of stats) tx[s.id] = s.netTx;
        if (prev && now > prev.t) {
            const dt = (now - prev.t) / 1000;
            setRates((old) => {
                const next = { ...old };
                for (const s of stats) {
                    const p = prev.tx[s.id];
                    if (p === undefined) continue;
                    const delta = s.netTx - p;
                    if (delta < 0) continue; // counter reset (restart) — skip this tick
                    const rate = delta / dt;
                    next[s.id] = old[s.id] === undefined ? rate : old[s.id] * 0.6 + rate * 0.4;
                }
                return next;
            });
        }
        prevRef.current = { tx, t: now };
    }, [stats]);

    const configured = model.computeMonthly > 0 || model.storageMonthly > 0 || model.egressGBMonthly > 0;
    const egressMonthly = model.egressGBMonthly * model.egressPerGB;
    const total = model.computeMonthly + model.storageMonthly + egressMonthly;

    const buckets = [
        { key: "compute", label: "Compute", amount: model.computeMonthly, color: "bg-sky-500", note: "instance-hours · fixed" },
        { key: "storage", label: "Storage", amount: model.storageMonthly, color: "bg-amber-500", note: "EBS provisioned · fixed" },
        { key: "egress", label: "Data transfer", amount: egressMonthly, color: "bg-violet-500", note: `${model.egressGBMonthly} GB out · variable` },
    ];

    // Per-container egress split from current tx-rate share.
    const containers = stats ?? [];
    const totalRate = containers.reduce((a, s) => a + (rates[s.id] ?? 0), 0);
    const perContainer = containers
        .map((s) => {
            const share = totalRate > 0 ? (rates[s.id] ?? 0) / totalRate : 0;
            return { id: s.id, name: s.name, share, dollars: share * egressMonthly, rate: rates[s.id] ?? 0 };
        })
        .filter((c) => c.rate > 0)
        .sort((a, b) => b.share - a.share);

    return (
        <section className="mt-6">
            <div className="mb-3 flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <span className="rounded-lg bg-muted p-2 text-foreground">
                        <DollarSign className="h-4 w-4" aria-hidden="true" />
                    </span>
                    <div>
                        <h2 className="text-sm font-semibold">Cost attribution</h2>
                        <p className="text-xs text-muted-foreground">Rough monthly estimate · edit rates to match your bill</p>
                    </div>
                </div>
                <button onClick={() => setEditing((v) => !v)} className="inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-muted">
                    <Pencil className="h-3.5 w-3.5" />
                    Edit rates
                </button>
            </div>

            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
                {editing || !configured ? (
                    <RateForm
                        model={model}
                        onSave={(m) => {
                            setModel(m);
                            localStorage.setItem(STORAGE_KEY, JSON.stringify(m));
                            setEditing(false);
                        }}
                        onCancel={configured ? () => setEditing(false) : undefined}
                    />
                ) : (
                    <>
                        <div className="flex items-baseline justify-between">
                            <p className="text-2xl font-semibold tracking-tight">${total.toFixed(2)}<span className="ml-1 text-sm font-normal text-muted-foreground">/mo est.</span></p>
                            <p className="text-xs text-muted-foreground">
                                fixed <b className="text-foreground">${(model.computeMonthly + model.storageMonthly).toFixed(0)}</b> · variable <b className="text-foreground">${egressMonthly.toFixed(0)}</b>
                            </p>
                        </div>

                        {/* stacked bill bar */}
                        <div className="mt-4 flex h-3 overflow-hidden rounded-full bg-muted">
                            {buckets.map((b) => (
                                <div key={b.key} className={b.color} style={{ width: total > 0 ? `${(b.amount / total) * 100}%` : "0%" }} title={`${b.label}: $${b.amount.toFixed(2)}`} />
                            ))}
                        </div>
                        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-3">
                            {buckets.map((b) => (
                                <div key={b.key} className="flex items-center gap-2">
                                    <span className={`h-2.5 w-2.5 shrink-0 rounded-sm ${b.color}`} />
                                    <div className="min-w-0">
                                        <p className="text-xs font-medium text-foreground">
                                            {b.label} · ${b.amount.toFixed(2)}
                                            <span className="ml-1 text-muted-foreground">{total > 0 ? `${((b.amount / total) * 100).toFixed(0)}%` : "0%"}</span>
                                        </p>
                                        <p className="text-[11px] text-muted-foreground">{b.note}</p>
                                    </div>
                                </div>
                            ))}
                        </div>

                        {/* per-container egress attribution */}
                        <div className="mt-5 border-t border-border pt-4">
                            <p className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Data transfer by container (egress share)</p>
                            {egressMonthly <= 0 ? (
                                <p className="text-xs text-muted-foreground">Set a monthly egress figure above to attribute the ${egressMonthly.toFixed(0)} transfer cost.</p>
                            ) : perContainer.length === 0 ? (
                                <p className="text-xs text-muted-foreground">Measuring egress rates… (needs a couple of refresh cycles).</p>
                            ) : (
                                <div className="space-y-1.5">
                                    {perContainer.slice(0, 8).map((c) => (
                                        <div key={c.id} className="flex items-center gap-3 text-xs">
                                            <span className="w-40 shrink-0 truncate font-medium text-foreground">{c.name}</span>
                                            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                                                <div className="h-full rounded-full bg-violet-500" style={{ width: `${c.share * 100}%` }} />
                                            </div>
                                            <span className="w-10 text-right tabular-nums text-muted-foreground">{(c.share * 100).toFixed(0)}%</span>
                                            <span className="w-16 text-right tabular-nums font-medium text-foreground">${c.dollars.toFixed(2)}</span>
                                            <span className="w-20 text-right tabular-nums text-muted-foreground">{fmtBytes(c.rate)}/s</span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>

                        <div className="mt-4 flex items-start gap-1.5 rounded-md border border-amber-500/20 bg-amber-500/[0.06] px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
                            <Info className="mt-0.5 h-3 w-3 shrink-0" />
                            <span>
                                Rough. Compute + storage are fixed (same regardless of container load). Egress share is from <code>docker stats</code> tx rate, which
                                <b> includes internal traffic</b> (DB/cache/inter-container) that AWS does <b>not</b> bill — reconcile against CloudWatch <code>NetworkOut</code> for the true number.
                            </span>
                        </div>
                    </>
                )}
            </div>
        </section>
    );
}

function RateForm({ model, onSave, onCancel }: { model: CostModel; onSave: (m: CostModel) => void; onCancel?: () => void }) {
    const [m, setM] = useState(model);
    const field = (key: keyof CostModel, label: string, hint: string, step = "1") => (
        <label className="block">
            <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>
            <input
                type="number"
                step={step}
                min="0"
                value={Number.isFinite(m[key]) ? m[key] : 0}
                onChange={(e) => setM({ ...m, [key]: parseFloat(e.target.value) || 0 })}
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary"
            />
            <span className="mt-1 block text-[11px] text-muted-foreground">{hint}</span>
        </label>
    );
    return (
        <div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {field("computeMonthly", "Compute $/mo", "Instance hourly × 730 (e.g. t3.large ≈ $60)")}
                {field("storageMonthly", "Storage $/mo", "EBS provisioned GB × rate (gp3 ≈ $0.08/GB)")}
                {field("egressGBMonthly", "Egress GB/mo", "Billable GB out (CloudWatch NetworkOut, net of 100 GB free)")}
                {field("egressPerGB", "Egress $/GB", "Usually ~$0.09 to internet", "0.001")}
            </div>
            <div className="mt-4 flex justify-end gap-2">
                {onCancel ? (
                    <button onClick={onCancel} className="rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted">
                        Cancel
                    </button>
                ) : null}
                <button onClick={() => onSave(m)} className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:opacity-90">
                    Save rates
                </button>
            </div>
        </div>
    );
}
