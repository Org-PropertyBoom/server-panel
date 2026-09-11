import { runtime } from "../runtime";

export type ApiRoute = "apps" | "containers" | "login" | "session" | "settings" | "system" | "update";

export type ApiRouteMap = Record<ApiRoute, string>;

const rootApi: ApiRouteMap = {
  apps: "/post/apps",
  containers: "/post/containers",
  login: "/post/login",
  session: "/post/session",
  settings: "/post/settings",
  system: "/post/system",
  update: "/post/update",
};

const userApi: ApiRouteMap = {
  apps: "/api/apps",
  containers: "/api/containers",
  login: "/api/login",
  session: "/api/session",
  settings: "/api/settings",
  system: "/api/system",
  update: "/api/update",
};

const Api = {
  root: rootApi,
  user: userApi,

  get current(): ApiRouteMap {
    return runtime.isRoot ? rootApi : userApi;
  },
};

export default Api;
