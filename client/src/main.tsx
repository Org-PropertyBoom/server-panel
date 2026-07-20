import { AppProvider } from "./_contexts/app";
import Routes from "./routes";
import { BrowserRouter } from "react-router-dom";
import { useEffect, useState } from "react";
import { TerminalProvider, useTerminal } from "./_contexts/terminal";
import TerminalPanel from "./_components/terminal-panel";
import { Toaster } from "_layouts/_components/ui/sonner";

export default function Main() {
    return (
        <BrowserRouter>
            <AppProvider>
                <TerminalProvider>
                    <Routes />
                    <GlobalTerminal />
                    <Toaster />
                </TerminalProvider>
            </AppProvider>
        </BrowserRouter>
    );
}

function GlobalTerminal() {
    const { isOpen } = useTerminal();
    const [mounted, setMounted] = useState(false);
    useEffect(() => {
        if (isOpen) setMounted(true);
    }, [isOpen]);
    if (!mounted) return null;
    return (
        <div className={`fixed inset-x-0 bottom-0 z-50 md:left-[60px] ${isOpen ? "block" : "hidden"}`}>
            <TerminalPanel />
        </div>
    );
}
