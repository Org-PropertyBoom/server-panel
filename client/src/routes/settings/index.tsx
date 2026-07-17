import DashboardLayout from "_layouts/dashboard";

export default function SettingsRoute() {
    return (
        <DashboardLayout
            title="Settings"
            description="Configure server panel preferences and system defaults."
        >
            <div className="rounded-md border border-border bg-card p-4 text-sm text-muted-foreground">
                Settings will appear here as configuration options become available.
            </div>
        </DashboardLayout>
    );
}
