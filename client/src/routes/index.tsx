import { runtime } from "../runtime";
import LoginRoute from "./login";
import RootRoutes from "./root";
import UsersRoute from "./root/users";
import UserRoutes from "./user";
import FilesRoute from "./files";
import VHostsRoute from "./vhosts";
import ModulesRoute from "./modules";

export default function Routes() {
    if (isRoute("/login")) {
        return <LoginRoute />;
    }

    if (isRoute("/files")) {
        return <FilesRoute />;
    }

    if (isRoute("/vhosts")) {
        return <VHostsRoute />;
    }

    if (isRoute("/modules")) {
        return <ModulesRoute />;
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
