import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { runtime } from "../runtime";
import { getAppName, storeAppName } from "_utils/app-settings";
import Api from "_utils/api";
import { setColorModePreference, type ColorModePreference } from "_utils/color-mode";

type AppContextType = {
    isRoot: boolean;
    mode: "root" | "user";
    env: string;
    appName: string;
    setAppName: (appName: string) => void;
    headerApps: string[];
    setHeaderApps: (apps: string[]) => void;
    setDefaultColorMode: (mode: ColorModePreference) => void;
    settings: Record<string, string>;
    setSetting: (key: string, value: string) => void;
};

const AppContext = createContext<AppContextType | undefined>(undefined);

export function AppProvider({ children }: { children: ReactNode }) {
    const [appName, setCurrentAppName] = useState(getAppName);
    const [headerApps, setCurrentHeaderApps] = useState<string[]>([]);
    const [settings, setSettings] = useState<Record<string, string>>({});

    const saveSetting = (key: string, value: string) => {
        setSettings((current) => ({ ...current, [key]: value }));
        void fetch(Api.current.settings, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ key, value }),
        });
    };

    useEffect(() => {
        if (window.localStorage.getItem("is_logged_in") !== "true") return;
        void fetch(Api.current.settings)
            .then((response) => response.ok ? response.json() : null)
            .then((data) => {
                const settings = data?.settings as Record<string, string> | undefined;
                if (!settings) return;
                setSettings(settings);
                if (settings.general_app_name) {
                    setCurrentAppName(storeAppName(settings.general_app_name));
                }
                if (["system", "light", "dark"].includes(settings.general_color_mode)) {
                    setColorModePreference(settings.general_color_mode as ColorModePreference);
                }
                if (settings.apps_header) {
                    try {
                        setCurrentHeaderApps(JSON.parse(settings.apps_header));
                    } catch {}
                }
            });
    }, []);

    const setAppName = (value: string) => {
        setCurrentAppName(storeAppName(value));
        saveSetting("general_app_name", value.trim());
    };

    const setHeaderApps = (apps: string[]) => {
        setCurrentHeaderApps(apps);
        saveSetting("apps_header", JSON.stringify(apps));
    };

    const setDefaultColorMode = (mode: ColorModePreference) => {
        setColorModePreference(mode);
        saveSetting("general_color_mode", mode);
    };

    return (
        <AppContext.Provider
            value={{
                isRoot: runtime.isRoot,
                mode: runtime.mode,
                env: runtime.env,
                appName,
                setAppName,
                headerApps,
                setHeaderApps,
                setDefaultColorMode,
                settings,
                setSetting: saveSetting,
            }}
        >
            {children}
        </AppContext.Provider>
    );
}

export function useApp() {
    const context = useContext(AppContext);
    if (context === undefined) {
        throw new Error("useApp must be used within an AppProvider");
    }
    return context;
}
