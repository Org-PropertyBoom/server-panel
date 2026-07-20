import { useEffect, useState } from "react";
import { Toaster as SonnerToaster } from "sonner";

// Follows the `.dark` class that color-mode.ts toggles on <html>, so toasts match
// the panel theme regardless of which color-mode setter fired.
function useHtmlTheme(): "dark" | "light" {
    const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
    useEffect(() => {
        const el = document.documentElement;
        const observer = new MutationObserver(() => setDark(el.classList.contains("dark")));
        observer.observe(el, { attributes: true, attributeFilter: ["class"] });
        return () => observer.disconnect();
    }, []);
    return dark ? "dark" : "light";
}

export function Toaster() {
    const theme = useHtmlTheme();
    return (
        <SonnerToaster
            theme={theme}
            position="top-right"
            richColors
            closeButton
            toastOptions={{ duration: 4000 }}
        />
    );
}
