import { type ReactNode, useEffect, useState } from "react";

import { UserProvider, useUser } from "../_contexts/user";
import Sidebar from "./_components/sidebar";
import Header from "./_components/header";
import { useApp } from "../_contexts/app";
import { useTerminal } from "../_contexts/terminal";

type DashboardLayoutProps = {
    actions?: ReactNode;
    children: ReactNode;
    description?: string;
    title: string;
    fullWidth?: boolean;
    wide?: boolean;
};

function DashboardLayoutContent({
    actions,
    children,
    description,
    title,
    fullWidth,
    wide,
}: DashboardLayoutProps) {
    const { isLoggedIn } = useUser();
    const { appName } = useApp();
    const [isMobileOpen, setIsMobileOpen] = useState(false);
    const { isOpen: isTerminalOpen, openRoot } = useTerminal();

    useEffect(() => {
        if (!isLoggedIn) {
            window.location.href = "/login";
        }
    }, [isLoggedIn]);

    useEffect(() => {
        if (title) {
            document.title = `${title} | ${appName}`;
        } else {
            document.title = appName;
        }
    }, [appName, title]);

    return (
        <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
            {/* Desktop Sidebar */}
            <Sidebar className="hidden md:flex" isTerminalOpen={isTerminalOpen} onTerminalToggle={openRoot} />

            {/* Mobile Sidebar Overlay */}
            {isMobileOpen && (
                <div
                    className="fixed inset-0 z-40 bg-background/80 backdrop-blur-sm md:hidden"
                    onClick={() => setIsMobileOpen(false)}
                />
            )}

            {/* Mobile Sidebar Drawer */}
            <Sidebar
                className={`fixed bottom-0 top-0 left-0 z-50 transition-transform duration-300 md:hidden ${
                    isMobileOpen ? "translate-x-0" : "-translate-x-full"
                }`}
                isTerminalOpen={isTerminalOpen}
                onTerminalToggle={openRoot}
            />

            {/* Main Content Area */}
            <div className="flex flex-1 flex-col overflow-hidden">
                <Header title={appName} onMenuClick={() => setIsMobileOpen(true)} />

                <div className="relative flex flex-1 flex-col overflow-hidden">
                    {fullWidth ? (
                        <main className="flex-1 overflow-hidden h-full w-full">
                            {children}
                        </main>
                    ) : (
                        <main className="flex-1 overflow-y-auto px-6 py-8">
                            <div className={`mx-auto w-full ${wide ? "max-w-none" : "max-w-5xl"}`}>
                                {/* Page Header */}
                                <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                                    <div className="max-w-2xl space-y-2">
                                        <h2 className="text-3xl font-semibold tracking-tight">
                                            {title}
                                        </h2>
                                        {description ? (
                                            <p className="text-sm leading-6 text-muted-foreground">
                                                {description}
                                            </p>
                                        ) : null}
                                    </div>

                                    {actions ? (
                                        <div className="flex flex-wrap gap-3">{actions}</div>
                                    ) : null}
                                </div>

                                {/* Page Content */}
                                {children}
                            </div>
                        </main>
                    )}

                </div>
            </div>
        </div>
    );
}

export default function DashboardLayout(props: DashboardLayoutProps) {
    return (
        <UserProvider>
            <DashboardLayoutContent {...props} />
        </UserProvider>
    );
}
