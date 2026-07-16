import { runtime } from "../runtime";

export type ApiRoute = "apps" | "login" | "session" | "system";

export type ApiRouteMap = Record<ApiRoute, string>;

const rootApi: ApiRouteMap = {
  apps: "/post/apps",
  login: "/post/login",
  session: "/post/session",
  system: "/post/system",
};

const userApi: ApiRouteMap = {
  apps: "/api/apps",
  login: "/api/login",
  session: "/api/session",
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
