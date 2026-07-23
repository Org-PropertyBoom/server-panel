import SystemDashboard from "_components/system-dashboard";
import ContainerStatsPanel from "_components/container-stats";
import DashboardLayout from "_layouts/dashboard";
import { runtime } from "../../runtime";

export default function RootRoutes() {
    return (
        <DashboardLayout
            title="System overview"
            description={`${runtime.osName} · Real-time server resource overview.`}
        >
            <SystemDashboard />
            <ContainerStatsPanel />
        </DashboardLayout>
    );
}
