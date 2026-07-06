import { runtime } from "../../runtime";
import RootVHosts from "./root";
import UserVHosts from "./user";

export default function VHostsRoute() {
    if (runtime.isRoot) {
        return <RootVHosts />;
    }
    return <UserVHosts />;
}
