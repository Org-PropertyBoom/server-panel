import { runtime } from "../runtime";
import LoginRoute from "./login";
import RootRoutes from "./root";
import UsersRoute from "./root/users";
import UserRoutes from "./user";
import FilesRoute from "./files";

export default function Routes() {
    if (isRoute("/login")) {
        return <LoginRoute />;
    }

    if (isRoute("/files")) {
        return <FilesRoute />;
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
