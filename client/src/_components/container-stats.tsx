import { useEffect, useState } from "react";
import { Boxes } from "lucide-react";

type ContainerStat = {
    id: string;
    name: string;
    cpuPerc: number;
    memUsed: number;
    memLimit: number;
    memPerc: number;
    netRx: number;
    netTx: number;
    blockRead: number;
    blockWrite: number;
    pids: number;
};

// ContainerStatsPanel shows live per-container resource usage (docker stats) below
// the system cards. Root-only surface (the endpoint is /post/*). It polls on a
// slightly longer interval than the system cards because `docker stats` samples CPU
// over ~1-2s per call.
export default function ContainerStatsPanel() {
    const [stats, setStats] = useState<ContainerStat[] | null>(null);
    const [error, setError] = useState(false);

    useEffect(() => {
        let active = true;
        const load = async () => {
            try {
                const res = await fetch("/post/containers/stats", { cache: "no-store" });
                if (!res.ok) throw new Error("stats request failed");
                const data: { containers?: ContainerStat[] } = await res.json();
                if (active) {
                    setStats(data.containers ?? []);
                    setError(false);
                }
            } catch {
                if (active) setError(true);
            }
        };
        load();
        const interval = window.setInterval(load, 8000);
        return () => {
            active = false;
            window.clearInterval(interval);
        };
    }, []);

    const running = stats?.length ?? 0;
    const totalMem = stats?.reduce((a, s) => a + s.memUsed, 0) ?? 0;
    const totalCpu = stats?.reduce((a, s) => a + s.cpuPerc, 0) ?? 0;

    return (
        <section className="mt-6">
            <div className="mb-3 flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <span className="rounded-lg bg-muted p-2 text-foreground">
                        <Boxes className="h-4 w-4" aria-hidden="true" />
                    </span>
                    <div>
                        <h2 className="text-sm font-semibold">Containers</h2>
                        <p className="text-xs text-muted-foreground">Live resource usage · refreshes every 8s</p>
                    </div>
                </div>
                {stats ? (
                    <p className="text-xs text-muted-foreground">
                        {running} running · <b className="text-foreground">{fmtBytes(totalMem)}</b> RAM · <b className="text-foreground">{totalCpu.toFixed(1)}%</b> CPU
                    </p>
                ) : null}
            </div>

            <div className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
                {!stats ? (
                    <div className="divide-y divide-border">
                        {Array.from({ length: 4 }).map((_, i) => (
                            <div key={i} className="h-11 animate-pulse bg-muted/40" />
                        ))}
                        {error ? <p className="px-4 py-3 text-sm text-destructive">Unable to load container stats (is Docker running?).</p> : null}
                    </div>
                ) : stats.length === 0 ? (
                    <p className="px-4 py-6 text-center text-sm text-muted-foreground">No running containers.</p>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[720px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-2.5 font-medium">Container</th>
                                    <th className="px-4 py-2.5 font-medium">CPU</th>
                                    <th className="px-4 py-2.5 font-medium">Memory</th>
                                    <th className="px-4 py-2.5 font-medium">Network I/O</th>
                                    <th className="px-4 py-2.5 font-medium">Block I/O</th>
                                    <th className="px-4 py-2.5 text-right font-medium">PIDs</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {stats.map((s) => (
                                    <tr key={s.id} className="hover:bg-muted/30">
                                        <td className="px-4 py-2.5 font-medium text-foreground">{s.name}</td>
                                        <td className="px-4 py-2.5">
                                            <div className="flex items-center gap-2">
                                                <span className="w-12 tabular-nums text-foreground">{s.cpuPerc.toFixed(1)}%</span>
                                                <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                                                    <div className="h-full rounded-full bg-primary" style={{ width: `${Math.min(100, s.cpuPerc)}%` }} />
                                                </div>
                                            </div>
                                        </td>
                                        <td className="px-4 py-2.5">
                                            <span className="text-foreground">{fmtBytes(s.memUsed)}</span>
                                            <span className="text-muted-foreground"> / {fmtBytes(s.memLimit)}</span>
                                            <span className="ml-1 text-muted-foreground/70">({s.memPerc.toFixed(1)}%)</span>
                                        </td>
                                        <td className="px-4 py-2.5 tabular-nums text-muted-foreground">
                                            ↓ {fmtBytes(s.netRx)} · ↑ {fmtBytes(s.netTx)}
                                        </td>
                                        <td className="px-4 py-2.5 tabular-nums text-muted-foreground">
                                            r {fmtBytes(s.blockRead)} · w {fmtBytes(s.blockWrite)}
                                        </td>
                                        <td className="px-4 py-2.5 text-right tabular-nums text-foreground">{s.pids}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
        </section>
    );
}

function fmtBytes(bytes: number) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const value = bytes / 1024 ** unit;
    return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}
