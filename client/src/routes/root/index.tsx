import SystemDashboard from "_components/system-dashboard";
import DashboardLayout from "_layouts/dashboard";
import { runtime } from "../../runtime";

export default function RootRoutes() {
    return (
        <DashboardLayout
            title="System overview"
            description={`${runtime.osName} · Real-time server resource overview.`}
        >
            <SystemDashboard />
        </DashboardLayout>
    );
}
