import { runtime } from "../runtime";
import LoginRoute from "./login";
import RootRoutes from "./root";
import UsersRoute from "./root/users";
import UserRoutes from "./user";

export default function Routes() {
    if (isRoute("/login")) {
        return <LoginRoute />;
    }

    if (runtime.isRoot) {
        if (isRoute("/users")) {
            return <UsersRoute />;
        }

        return <RootRoutes />;
    }

    return <UserRoutes />;
}

function isRoute(pathname: string) {
    return trimTrailingSlash(window.location.pathname) === pathname;
}

function trimTrailingSlash(pathname: string) {
    if (pathname === "/") {
        return pathname;
    }

    return pathname.replace(/\/+$/, "");
}
