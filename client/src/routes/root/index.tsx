import SystemDashboard from "_components/system-dashboard";
import ContainerStatsPanel, { useContainerStats } from "_components/container-stats";
import CostAttributionPanel from "_components/cost-attribution";
import DashboardLayout from "_layouts/dashboard";
import { runtime } from "../../runtime";

export default function RootRoutes() {
    const { stats, error } = useContainerStats(8000);
    return (
        <DashboardLayout
            title="System overview"
            description={`${runtime.osName} · Real-time server resource overview.`}
        >
            <SystemDashboard />
            <ContainerStatsPanel stats={stats} error={error} />
            <CostAttributionPanel stats={stats} />
        </DashboardLayout>
    );
}
