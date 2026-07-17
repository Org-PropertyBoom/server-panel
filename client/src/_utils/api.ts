import { runtime } from "../runtime";

export type ApiRoute = "apps" | "login" | "session" | "settings" | "system";

export type ApiRouteMap = Record<ApiRoute, string>;

const rootApi: ApiRouteMap = {
  apps: "/post/apps",
  login: "/post/login",
  session: "/post/session",
  settings: "/post/settings",
  system: "/post/system",
};

const userApi: ApiRouteMap = {
  apps: "/api/apps",
  login: "/api/login",
  session: "/api/session",
  settings: "/api/settings",
  system: "/api/system",
};

const Api = {
  root: rootApi,
  user: userApi,

  get current(): ApiRouteMap {
    return runtime.isRoot ? rootApi : userApi;
  },
};

export default Api;
