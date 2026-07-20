import { Link } from "react-router-dom";
import { Boxes, Cpu, Database, Settings, User } from "lucide-react";

import { runtime } from "../../runtime";

export const availableApps = [
    ["caddy", "Caddy"],
    ["nginx", "Nginx"],
    ["mariadb", "MariaDB"],
    ["redis", "Redis"],
    ["docker", "Docker"],
    ["podman", "Podman"],
    ["node", "Node.js"],
    ["php", "PHP"],
] as const;

type SettingsSection = "general" | "users" | "apps" | "data-sources";

export default function SettingsSidebar({ section, app }: { section: SettingsSection; app?: string }) {
    return (
        <aside className="flex h-full flex-col gap-1 overflow-y-auto border-r border-border bg-card/60 p-2">
            <SettingsNavItem active={section === "general"} href="/settings/general" icon={Settings} label="General Settings" />
            <SettingsNavItem active={section === "users"} href="/settings/users" icon={User} label="Users Settings" />
            <SettingsNavItem active={section === "apps" && !app} href="/settings/apps" icon={Boxes} label="Apps Settings" />
            {runtime.isRoot ? (
                <SettingsNavItem active={section === "data-sources"} href="/settings/data-sources" icon={Database} label="Data Sources" />
            ) : null}
            {section === "apps" ? (
                <nav className="ml-5 border-l border-border py-1 pl-2">
                    {availableApps.map(([name, label]) => (
                        <Link
                            key={name}
                            to={`/settings/apps/${encodeURIComponent(name)}`}
                            className={`flex items-center gap-2 px-2 py-1.5 text-[11px] ${app === name ? "font-semibold text-primary" : "text-muted-foreground hover:text-foreground"}`}
                        >
                            <Cpu className="h-3.5 w-3.5" />
                            {label}
                        </Link>
                    ))}
                </nav>
            ) : null}
        </aside>
    );
}

function SettingsNavItem({ active, href, icon: Icon, label }: {
    active: boolean;
    href: string;
    icon: typeof Settings;
    label: string;
}) {
    return (
        <Link
            to={href}
            className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-xs font-semibold ${active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
        >
            <Icon className="h-4 w-4" />
            {label}
        </Link>
    );
}
