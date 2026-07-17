import { useEffect, useState } from "react";
import { ArrowDown, ArrowUp, Boxes, GripVertical, Plus, Settings, User, X } from "lucide-react";

import { useApp } from "_contexts/app";
import DashboardLayout from "_layouts/dashboard";
import { defaultAppName } from "_utils/app-settings";
import Api from "_utils/api";
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

type SettingsSection = "general" | "users" | "apps";

export default function SettingsRoute() {
    const { appName, setAppName, headerApps, setHeaderApps, setDefaultColorMode, settings, setSetting } = useApp();
    const section = getSettingsSection();
    const [appNameDraft, setAppNameDraft] = useState(appName);
    const [colorMode, setCurrentColorMode] = useState<ColorModePreference>(getColorModePreference);
    const [installedApps, setInstalledApps] = useState<string[]>([]);
    const [appsLoading, setAppsLoading] = useState(true);
    const [draggedApp, setDraggedApp] = useState<string | null>(null);
    const [defaultShell, setDefaultShell] = useState("/bin/bash");
    const [homeBase, setHomeBase] = useState("/home");

    useEffect(() => {
        const syncColorMode = () => setCurrentColorMode(getColorModePreference());
        window.addEventListener("vps-color-mode-change", syncColorMode);
        return () => window.removeEventListener("vps-color-mode-change", syncColorMode);
    }, []);

    useEffect(() => setAppNameDraft(appName), [appName]);

    useEffect(() => {
        setDefaultShell(settings.users_default_shell || "/bin/bash");
        setHomeBase(settings.users_home_base || "/home");
    }, [settings.users_default_shell, settings.users_home_base]);

    useEffect(() => {
        void fetch(Api.current.apps, { cache: "no-store" })
            .then((response) => response.ok ? response.json() : null)
            .then((data) => {
                const installed = (data?.apps as Array<{ name: string; installed: boolean }> | undefined)
                    ?.filter((app) => app.installed)
                    .map((app) => app.name) ?? [];
                setInstalledApps(installed);
            })
            .catch(() => setInstalledApps([]))
            .finally(() => setAppsLoading(false));
    }, []);

    const saveAppName = () => {
        const value = appNameDraft.trim() || defaultAppName;
        setAppName(value);
        setAppNameDraft(value);
    };

    const changeColorMode = (preference: ColorModePreference) => {
        setCurrentColorMode(preference);
        setDefaultColorMode(preference);
    };

    const moveHeaderApp = (name: string, offset: number) => {
        const index = headerApps.indexOf(name);
        const nextIndex = index + offset;
        if (index < 0 || nextIndex < 0 || nextIndex >= headerApps.length) return;
        const next = [...headerApps];
        [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
        setHeaderApps(next);
    };

    const dropHeaderApp = (target: string) => {
        if (!draggedApp || draggedApp === target) return;
        const next = headerApps.filter((app) => app !== draggedApp);
        next.splice(next.indexOf(target), 0, draggedApp);
        setHeaderApps(next);
        setDraggedApp(null);
    };

    return (
        <DashboardLayout title="Settings" fullWidth>
            <div className="grid h-full grid-cols-1 overflow-hidden md:grid-cols-[240px_1fr]">
                <aside className="flex h-full flex-col gap-1 border-r border-border bg-card/60 p-2">
                    <SettingsNavItem active={section === "general"} href="/settings/general" icon={Settings} label="General Settings" />
                    <SettingsNavItem active={section === "users"} href="/settings/users" icon={User} label="Users Settings" />
                    <SettingsNavItem active={section === "apps"} href="/settings/apps" icon={Boxes} label="Apps Settings" />
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
                    ) : section === "users" ? (
                        <div className="mx-auto max-w-2xl space-y-6">
                            <h2 className="text-lg font-semibold">Users Settings</h2>
                            <div className="divide-y divide-border rounded-md border border-border bg-card">
                                <div className="grid gap-3 p-4 sm:grid-cols-[180px_1fr] sm:items-center">
                                    <label htmlFor="default-shell" className="text-sm font-medium">Default Shell</label>
                                    <input
                                        id="default-shell"
                                        value={defaultShell}
                                        onChange={(event) => setDefaultShell(event.target.value)}
                                        onBlur={() => setSetting("users_default_shell", defaultShell.trim() || "/bin/bash")}
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </div>
                                <div className="grid gap-3 p-4 sm:grid-cols-[180px_1fr] sm:items-center">
                                    <label htmlFor="home-base" className="text-sm font-medium">Home Base</label>
                                    <input
                                        id="home-base"
                                        value={homeBase}
                                        onChange={(event) => setHomeBase(event.target.value)}
                                        onBlur={() => setSetting("users_home_base", homeBase.trim() || "/home")}
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </div>
                                <label className="flex items-center justify-between gap-4 p-4">
                                    <span className="text-sm font-medium">Create Home Directory</span>
                                    <input
                                        type="checkbox"
                                        checked={(settings.users_create_home ?? "true") === "true"}
                                        onChange={(event) => setSetting("users_create_home", String(event.target.checked))}
                                        className="h-4 w-4 rounded border-input"
                                    />
                                </label>
                                <label className="flex items-center justify-between gap-4 p-4">
                                    <span className="text-sm font-medium">Auto Username</span>
                                    <input
                                        type="checkbox"
                                        checked={(settings.users_auto_username ?? "false") === "true"}
                                        onChange={(event) => setSetting("users_auto_username", String(event.target.checked))}
                                        className="h-4 w-4 rounded border-input"
                                    />
                                </label>
                            </div>
                        </div>
                    ) : (
                        <div className="mx-auto max-w-2xl space-y-6">
                            <h2 className="text-lg font-semibold">Apps Settings</h2>
                            <div className="grid gap-4 md:grid-cols-2">
                                <section className="overflow-hidden rounded-md border border-border bg-card">
                                    <h3 className="border-b border-border px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                                        Installed Apps
                                    </h3>
                                    <div className="divide-y divide-border">
                                    {appsLoading ? (
                                        <p className="p-4 text-xs text-muted-foreground">Loading installed apps...</p>
                                    ) : availableApps.filter(([name]) => installedApps.includes(name)).length === 0 ? (
                                        <p className="p-4 text-xs text-muted-foreground">No supported apps detected.</p>
                                    ) : availableApps.filter(([name]) => installedApps.includes(name)).map(([name, label]) => {
                                        const added = headerApps.includes(name);
                                        return (
                                            <div key={name} className="flex items-center justify-between gap-3 p-3">
                                                <span className="text-sm font-medium">{label}</span>
                                                <button
                                                    type="button"
                                                    disabled={added}
                                                    onClick={() => setHeaderApps([...headerApps, name])}
                                                    className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border hover:bg-muted disabled:opacity-40"
                                                    aria-label={`Add ${label} to header`}
                                                    title={added ? "Already added" : "Add to header"}
                                                >
                                                    <Plus className="h-3.5 w-3.5" />
                                                </button>
                                            </div>
                                        );
                                    })}
                                    </div>
                                </section>

                                <section className="overflow-hidden rounded-md border border-border bg-card">
                                    <h3 className="border-b border-border px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                                        Header Apps
                                    </h3>
                                    <div className="divide-y divide-border">
                                    {headerApps.length === 0 ? (
                                        <p className="p-4 text-xs text-muted-foreground">No apps added to the header.</p>
                                    ) : headerApps.map((name, index) => {
                                        const label = availableApps.find(([app]) => app === name)?.[1] ?? name;
                                    return (
                                        <div
                                            key={name}
                                            draggable
                                            onDragStart={() => setDraggedApp(name)}
                                            onDragOver={(event) => event.preventDefault()}
                                            onDrop={() => dropHeaderApp(name)}
                                            className="flex items-center gap-2 p-3"
                                        >
                                            <GripVertical className="h-4 w-4 cursor-grab text-muted-foreground" />
                                            <div className="min-w-0 flex-1">
                                                <span className="text-sm font-medium">{label}</span>
                                            </div>
                                            <button
                                                type="button"
                                                disabled={index === 0}
                                                onClick={() => moveHeaderApp(name, -1)}
                                                className="inline-flex h-7 w-7 items-center justify-center rounded hover:bg-muted disabled:opacity-30"
                                                aria-label={`Move ${label} up`}
                                            >
                                                <ArrowUp className="h-3.5 w-3.5" />
                                            </button>
                                            <button type="button" disabled={index === headerApps.length - 1} onClick={() => moveHeaderApp(name, 1)} className="inline-flex h-7 w-7 items-center justify-center rounded hover:bg-muted disabled:opacity-30" aria-label={`Move ${label} down`}>
                                                <ArrowDown className="h-3.5 w-3.5" />
                                            </button>
                                            <button type="button" onClick={() => setHeaderApps(headerApps.filter((app) => app !== name))} className="inline-flex h-7 w-7 items-center justify-center rounded text-destructive hover:bg-destructive/10" aria-label={`Remove ${label} from header`}>
                                                <X className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    );
                                })}
                                    </div>
                                </section>
                            </div>
                        </div>
                    )}
                </main>
            </div>
        </DashboardLayout>
    );
}

function getSettingsSection(): SettingsSection {
    const section = window.location.pathname.split("/")[2];
    return section === "users" || section === "apps" ? section : "general";
}

function SettingsNavItem({ active, href, icon: Icon, label }: {
    active: boolean;
    href: string;
    icon: typeof Settings;
    label: string;
}) {
    return (
        <a
            href={href}
            className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-xs font-semibold ${
                active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
        >
            <Icon className="h-4 w-4" />
            {label}
        </a>
    );
}
