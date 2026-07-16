import { useEffect, useState, type ElementType } from "react";
import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react";

import Api from "_utils/api";

type SystemStatus = {
    cpu: { cores: number; model: string; usage: number };
    memory: { total: number; used: number; usage: number };
    storage: { total: number; used: number; usage: number };
    network: { received: number; sent: number };
};

type MetricCardProps = {
    icon: ElementType;
    title: string;
    value: string;
    description: string;
    progress?: number;
};

function MetricCard({ icon: Icon, title, value, description, progress }: MetricCardProps) {
    return (
        <article className="rounded-xl border border-border bg-card p-5 shadow-sm">
            <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-muted-foreground">{title}</p>
                <span className="rounded-lg bg-muted p-2 text-foreground">
                    <Icon className="h-4 w-4" aria-hidden="true" />
                </span>
            </div>
            <p className="mt-5 text-2xl font-semibold tracking-tight">{value}</p>
            {progress !== undefined ? (
                <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted">
                    <div
                        className="h-full rounded-full bg-primary transition-all duration-500"
                        style={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
                    />
                </div>
            ) : null}
            <p className="mt-3 truncate text-xs text-muted-foreground" title={description}>
                {description}
            </p>
        </article>
    );
}

export default function SystemDashboard() {
    const [status, setStatus] = useState<SystemStatus | null>(null);
    const [error, setError] = useState(false);

    useEffect(() => {
        let active = true;

        const load = async () => {
            try {
                const response = await fetch(Api.current.system);
                if (!response.ok) throw new Error("system request failed");
                const data: SystemStatus = await response.json();
                if (active) {
                    setStatus(data);
                    setError(false);
                }
            } catch {
                if (active) setError(true);
            }
        };

        load();
        const interval = window.setInterval(load, 5000);
        return () => {
            active = false;
            window.clearInterval(interval);
        };
    }, []);

    if (!status) {
        return (
            <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-busy="true">
                {Array.from({ length: 4 }).map((_, index) => (
                    <div key={index} className="h-44 animate-pulse rounded-xl border border-border bg-muted/50" />
                ))}
                {error ? <p className="col-span-full text-sm text-destructive">Unable to load system information.</p> : null}
            </section>
        );
    }

    return (
        <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard
                icon={Cpu}
                title="CPU"
                value={`${status.cpu.usage.toFixed(1)}%`}
                progress={status.cpu.usage}
                description={`${status.cpu.cores} cores · ${status.cpu.model}`}
            />
            <MetricCard
                icon={MemoryStick}
                title="RAM"
                value={`${formatBytes(status.memory.used)} / ${formatBytes(status.memory.total)}`}
                progress={status.memory.usage}
                description={`${status.memory.usage.toFixed(1)}% in use`}
            />
            <MetricCard
                icon={HardDrive}
                title="Storage"
                value={`${formatBytes(status.storage.used)} / ${formatBytes(status.storage.total)}`}
                progress={status.storage.usage}
                description={`${status.storage.usage.toFixed(1)}% used on /`}
            />
            <MetricCard
                icon={Network}
                title="Network"
                value={`↓ ${formatBytes(status.network.received)}`}
                description={`Sent ↑ ${formatBytes(status.network.sent)}`}
            />
        </section>
    );
}

function formatBytes(bytes: number) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const value = bytes / 1024 ** unit;
    return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}
