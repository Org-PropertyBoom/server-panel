import { useEffect, useState } from "react";
import { Boxes, Plus, Settings, X } from "lucide-react";

import { useApp } from "_contexts/app";
import DashboardLayout from "_layouts/dashboard";
import { defaultAppName } from "_utils/app-settings";
import {
    getColorModePreference,
    type ColorModePreference,
} from "_utils/color-mode";

const availableApps = [
    ["nginx", "Nginx"],
    ["mariadb", "MariaDB"],
    ["redis", "Redis"],
    ["docker", "Docker"],
    ["podman", "Podman"],
    ["node", "Node.js"],
    ["php", "PHP"],
] as const;

type SettingsSection = "general" | "apps";

export default function SettingsRoute() {
    const { appName, setAppName, headerApps, setHeaderApps, setDefaultColorMode } = useApp();
    const [section, setSection] = useState<SettingsSection>("general");
    const [appNameDraft, setAppNameDraft] = useState(appName);
    const [colorMode, setCurrentColorMode] = useState<ColorModePreference>(getColorModePreference);

    useEffect(() => {
        const syncColorMode = () => setCurrentColorMode(getColorModePreference());
        window.addEventListener("vps-color-mode-change", syncColorMode);
        return () => window.removeEventListener("vps-color-mode-change", syncColorMode);
    }, []);

    useEffect(() => setAppNameDraft(appName), [appName]);

    const saveAppName = () => {
        const value = appNameDraft.trim() || defaultAppName;
        setAppName(value);
        setAppNameDraft(value);
    };

    const changeColorMode = (preference: ColorModePreference) => {
        setCurrentColorMode(preference);
        setDefaultColorMode(preference);
    };

    return (
        <DashboardLayout title="Settings" fullWidth>
            <div className="grid h-full grid-cols-1 overflow-hidden md:grid-cols-[240px_1fr]">
                <aside className="flex h-full flex-col gap-1 border-r border-border bg-card/60 p-2">
                    <SettingsNavItem active={section === "general"} icon={Settings} label="General Settings" onClick={() => setSection("general")} />
                    <SettingsNavItem active={section === "apps"} icon={Boxes} label="Apps Settings" onClick={() => setSection("apps")} />
                </aside>

                <main className="overflow-y-auto p-6">
                    {section === "general" ? (
                    <div className="mx-auto max-w-2xl space-y-6">
                        <div>
                            <h2 className="text-lg font-semibold">General Settings</h2>
                            <p className="mt-1 text-sm text-muted-foreground">Configure the panel identity and appearance.</p>
                        </div>

                        <div className="divide-y divide-border rounded-md border border-border bg-card">
                            <div className="grid gap-3 p-4 sm:grid-cols-[180px_1fr] sm:items-center">
                                <label htmlFor="app-name" className="text-sm font-medium">App Name</label>
                                <input
                                    id="app-name"
                                    value={appNameDraft}
                                    onChange={(event) => setAppNameDraft(event.target.value)}
                                    onBlur={saveAppName}
                                    onKeyDown={(event) => {
                                        if (event.key === "Enter") event.currentTarget.blur();
                                    }}
                                    className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                                />
                            </div>

                            <div className="grid gap-3 p-4 sm:grid-cols-[180px_1fr] sm:items-center">
                                <label htmlFor="color-mode" className="text-sm font-medium">Default Color Mode</label>
                                <select
                                    id="color-mode"
                                    value={colorMode}
                                    onChange={(event) => changeColorMode(event.target.value as ColorModePreference)}
                                    className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                                >
                                    <option value="system">System</option>
                                    <option value="light">Light</option>
                                    <option value="dark">Dark</option>
                                </select>
                            </div>
                        </div>
                    </div>
                    ) : (
                        <div className="mx-auto max-w-2xl space-y-6">
                            <div>
                                <h2 className="text-lg font-semibold">Apps Settings</h2>
                                <p className="mt-1 text-sm text-muted-foreground">Choose app shortcuts to display in the header.</p>
                            </div>
                            <div className="divide-y divide-border rounded-md border border-border bg-card">
                                {availableApps.map(([name, label]) => {
                                    const added = headerApps.includes(name);
                                    return (
                                        <div key={name} className="flex items-center justify-between gap-4 p-4">
                                            <div className="flex items-center gap-3">
                                                <Boxes className="h-4 w-4 text-muted-foreground" />
                                                <span className="text-sm font-medium">{label}</span>
                                            </div>
                                            <button
                                                type="button"
                                                onClick={() => setHeaderApps(added ? headerApps.filter((app) => app !== name) : [...headerApps, name])}
                                                className={`inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium ${
                                                    added
                                                        ? "border-destructive/30 text-destructive hover:bg-destructive/10"
                                                        : "border-border text-foreground hover:bg-muted"
                                                }`}
                                            >
                                                {added ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
                                                {added ? "Remove" : "Add"}
                                            </button>
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    )}
                </main>
            </div>
        </DashboardLayout>
    );
}

function SettingsNavItem({ active, icon: Icon, label, onClick }: {
    active: boolean;
    icon: typeof Settings;
    label: string;
    onClick: () => void;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-xs font-semibold ${
                active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
        >
            <Icon className="h-4 w-4" />
            {label}
        </button>
    );
}
