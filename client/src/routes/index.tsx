import { Route, Routes as RouterRoutes } from "react-router-dom";

import { runtime } from "../runtime";
import AgentRoute from "./agent";
import AppsRoute from "./apps";
import ContainersRoute from "./containers";
import APIsRoute from "./apis";
import FilesRoute from "./files";
import LoginRoute from "./login";
import RootRoutes from "./root";
import UsersRoute from "./root/users";
import SettingsRoute from "./settings";
import UserRoutes from "./user";
import VHostsRoute from "./vhosts";

export default function AppRoutes() {
    return (
        <RouterRoutes>
            <Route path="/login" element={<LoginRoute />} />
            <Route path="/files" element={<FilesRoute />} />
            <Route path="/vhosts" element={<VHostsRoute />} />
            <Route path="/apps" element={<AppsRoute />} />
            <Route path="/apps/:app" element={<AppsRoute />} />
            <Route path="/containers" element={<ContainersRoute />} />
            <Route path="/apis" element={<APIsRoute />} />
            <Route path="/agent" element={<AgentRoute />} />
            <Route path="/settings" element={<SettingsRoute />} />
            <Route path="/settings/:section" element={<SettingsRoute />} />
            {runtime.isRoot ? (
                <>
                    <Route path="/users" element={<UsersRoute />} />
                    <Route path="/users/:username" element={<UsersRoute />} />
                    <Route path="/users/:username/:section" element={<UsersRoute />} />
                    <Route path="*" element={<RootRoutes />} />
                </>
            ) : (
                <Route path="*" element={<UserRoutes />} />
            )}
        </RouterRoutes>
    );
}
