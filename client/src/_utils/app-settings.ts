export const defaultAppName = "Ppt Server Panel";

const appNameStorageKey = "vps-app-name";

export function getAppName() {
    return window.localStorage.getItem(appNameStorageKey)?.trim() || defaultAppName;
}

export function storeAppName(appName: string) {
    const value = appName.trim() || defaultAppName;
    window.localStorage.setItem(appNameStorageKey, value);
    return value;
}
