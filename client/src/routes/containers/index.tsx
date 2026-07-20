import { useCallback, useEffect, useState } from "react";
import { Container as ContainerIcon, RefreshCw } from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";

type ContainerRecord = {
    id: string;
    name: string;
    image: string;
    command?: string;
    engine: "docker" | "podman";
    owner: string;
    state: string;
    status: string;
    createdAt?: string;
    ports: string[];
};

export default function ContainersRoute() {
    const [containers, setContainers] = useState<ContainerRecord[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");

    const loadContainers = useCallback(async () => {
        setLoading(true);
        setError("");
        try {
            const response = await fetch(Api.current.containers, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load containers");
            const data: { containers?: ContainerRecord[] } = await response.json();
            setContainers(data.containers ?? []);
        } catch (loadError) {
            setError(loadError instanceof Error ? loadError.message : "Failed to load containers");
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadContainers();
    }, [loadContainers]);

    return (
        <DashboardLayout
            title="Containers"
            description="View Docker system containers and isolated rootless Podman containers."
            actions={
                <Button variant="outline" size="sm" className="gap-2" onClick={loadContainers} disabled={loading}>
                    <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                    Refresh
                </Button>
            }
        >
            {error ? (
                <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                    {error}
                </div>
            ) : null}

            {!error && !loading && containers.length === 0 ? (
                <div className="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-border text-center">
                    <ContainerIcon className="mb-3 h-9 w-9 text-muted-foreground/40" />
                    <p className="text-sm font-medium text-foreground">No containers found</p>
                    <p className="mt-1 text-xs text-muted-foreground">Docker and Podman are available from their respective user terminals.</p>
                </div>
            ) : null}

            {containers.length > 0 ? (
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[900px] text-left text-xs">
                            <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                <tr>
                                    <th className="px-4 py-3 font-medium">Container</th>
                                    <th className="px-4 py-3 font-medium">Engine</th>
                                    <th className="px-4 py-3 font-medium">Owner</th>
                                    <th className="px-4 py-3 font-medium">Image</th>
                                    <th className="px-4 py-3 font-medium">State</th>
                                    <th className="px-4 py-3 font-medium">Ports</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {containers.map((container) => (
                                    <tr key={`${container.engine}:${container.owner}:${container.id}`} className="hover:bg-muted/30">
                                        <td className="px-4 py-3">
                                            <p className="font-medium text-foreground">{container.name || container.id.slice(0, 12)}</p>
                                            <code className="mt-0.5 block text-[10px] text-muted-foreground">{container.id.slice(0, 12)}</code>
                                        </td>
                                        <td className="px-4 py-3">
                                            <span className="rounded border border-border bg-muted px-2 py-1 font-medium capitalize text-foreground">{container.engine}</span>
                                        </td>
                                        <td className="px-4 py-3 font-medium text-foreground">{container.owner}</td>
                                        <td className="max-w-64 truncate px-4 py-3 text-foreground" title={container.image}>{container.image || "—"}</td>
                                        <td className="px-4 py-3">
                                            <div className="flex items-center gap-2">
                                                <span className={`h-2 w-2 rounded-full ${container.state.toLowerCase() === "running" ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
                                                <span className="text-foreground">{container.status || container.state || "Unknown"}</span>
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 text-muted-foreground">
                                            {container.ports?.length ? container.ports.join(", ") : "—"}
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            ) : null}
        </DashboardLayout>
    );
}
