import { useState } from "react";
import {
    LayoutDashboard,
    Server,
    Terminal,
    Folder,
    LogOut,
    User,
    Users,
    Globe,
    Boxes,
    Bot,
    Settings,
} from "lucide-react";
import { useUser } from "../../_contexts/user";
import { runtime } from "../../runtime";

type SidebarProps = {
    className?: string;
    isTerminalOpen?: boolean;
    onTerminalToggle?: () => void;
};

export default function Sidebar({ className = "", isTerminalOpen, onTerminalToggle }: SidebarProps) {
    const { logout } = useUser();
    const [isUserMenuOpen, setIsUserMenuOpen] = useState(false);

    const topMenuItems = [
        { icon: LayoutDashboard, label: "Dashboard", href: "/" },
        { icon: Bot, label: "Agent", href: "/agent" },
        { icon: Users, label: "Users", href: "/users" },
        { icon: Globe, label: "VHosts", href: "/vhosts" },
        { icon: Folder, label: "Files", href: "/files" },
        { icon: Boxes, label: "Apps", href: "/apps" },
    ];

    const handleLogoutClick = (e: React.MouseEvent<HTMLButtonElement>) => {
        e.preventDefault();
        logout();
    };

    return (
        <aside className={`flex h-screen w-[60px] flex-col border-r border-border bg-card text-card-foreground z-30 ${className}`}>
            {/* Logo area */}
            <div className="flex h-14 items-center justify-center border-b border-border">
                <div className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-primary/10 text-primary">
                    <Server className="h-5 w-5" />
                </div>
            </div>

            {/* Navigation links */}
            <nav className="flex-1 flex flex-col items-center gap-4 px-2 py-6">
                {topMenuItems.map((item) => {
                    const Icon = item.icon;
                    const isActive = window.location.pathname === item.href;
                    return (
                        <a
                            key={item.label}
                            href={item.href}
                            className={`group relative flex h-10 w-10 items-center justify-center rounded-md transition-colors ${
                                isActive
                                    ? "bg-primary text-primary-foreground"
                                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                            }`}
                        >
                            <Icon className="h-5 w-5 shrink-0" />
                            {/* Hover Tooltip */}
                            <span className="pointer-events-none absolute left-14 top-1/2 -translate-y-1/2 translate-x-1 opacity-0 group-hover:translate-x-0 group-hover:opacity-100 transition-all duration-150 rounded bg-popover px-2.5 py-1.5 text-xs font-medium text-popover-foreground shadow-md border border-border whitespace-nowrap z-50">
                                {item.label}
                            </span>
                        </a>
                    );
                })}

                {/* Spacer to push Terminal to bottom */}
                <div className="flex-1" />

                <a
                    href="/settings"
                    className={`group relative flex h-10 w-10 items-center justify-center rounded-md transition-colors ${
                        window.location.pathname === "/settings"
                            ? "bg-primary text-primary-foreground"
                            : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                >
                    <Settings className="h-5 w-5 shrink-0" />
                    <span className="pointer-events-none absolute left-14 top-1/2 z-50 -translate-y-1/2 translate-x-1 whitespace-nowrap rounded border border-border bg-popover px-2.5 py-1.5 text-xs font-medium text-popover-foreground opacity-0 shadow-md transition-all duration-150 group-hover:translate-x-0 group-hover:opacity-100">
                        Settings
                    </span>
                </a>

                {/* Terminal Menu Item */}
                {runtime.isRoot && (
                    <a
                        href="#terminal"
                        onClick={(e) => {
                            e.preventDefault();
                            if (onTerminalToggle) onTerminalToggle();
                        }}
                        className={`group relative flex h-10 w-10 items-center justify-center rounded-md transition-colors ${
                            isTerminalOpen
                                ? "bg-primary text-primary-foreground"
                                : "text-muted-foreground hover:bg-muted hover:text-foreground"
                        }`}
                    >
                        <Terminal className="h-5 w-5 shrink-0" />
                        {/* Hover Tooltip */}
                        <span className="pointer-events-none absolute left-14 top-1/2 -translate-y-1/2 translate-x-1 opacity-0 group-hover:translate-x-0 group-hover:opacity-100 transition-all duration-150 rounded bg-popover px-2.5 py-1.5 text-xs font-medium text-popover-foreground shadow-md border border-border whitespace-nowrap z-50">
                            Terminal
                        </span>
                    </a>
                )}
            </nav>

            {/* User Toggle & Dropdown */}
            <div className="border-t border-border p-2.5 flex justify-center relative">
                <button
                    onClick={() => setIsUserMenuOpen(!isUserMenuOpen)}
                    className={`flex h-9 w-9 items-center justify-center rounded-full bg-muted border border-border text-muted-foreground hover:text-foreground transition-colors hover:bg-accent ${
                        isUserMenuOpen ? "bg-accent text-foreground" : ""
                    }`}
                    title="User Account"
                >
                    <User className="h-5 w-5" />
                </button>

                {isUserMenuOpen && (
                    <>
                        {/* Click-outside backdrop */}
                        <div
                            className="fixed inset-0 z-40 cursor-default"
                            onClick={() => setIsUserMenuOpen(false)}
                        />
                        {/* Dropdown Box */}
                        <div className="absolute bottom-12 left-14 z-50 w-48 rounded-md border border-border bg-popover py-1.5 shadow-lg text-popover-foreground animate-in fade-in slide-in-from-bottom-2 duration-100">
                            <div className="px-3 py-2 border-b border-border">
                                <p className="text-xs font-semibold truncate">
                                    {runtime.username || "System User"}
                                </p>
                                <p className="text-[10px] text-muted-foreground capitalize mt-0.5">
                                    {runtime.mode} session
                                </p>
                            </div>
                            <button
                                onClick={handleLogoutClick}
                                className="flex w-full items-center gap-2 px-3 py-2 text-sm text-destructive hover:bg-destructive/10 transition-colors text-left font-medium"
                            >
                                <LogOut className="h-4 w-4 shrink-0" />
                                Logout
                            </button>
                        </div>
                    </>
                )}
            </div>
        </aside>
    );
}
