// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { readRoute, routeURL, type AdminRoute } from "./routes";
const current = () =>
  readRoute(window.location.pathname, window.location.search);
export function useRoute() {
  const [route, setRoute] = useState(current);
  useEffect(() => {
    const initial = current();
    if (!initial.notFound)
      window.history.replaceState(
        { authAdmin: initial },
        "",
        routeURL(initial),
      );
    const restore = () => setRoute(current());
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  const navigate = (next: AdminRoute, replace = false) => {
    const url = routeURL(next);
    if (url !== window.location.pathname + window.location.search) {
      window.history[replace ? "replaceState" : "pushState"](
        { authAdmin: next },
        "",
        url,
      );
    }
    setRoute(next);
  };
  return [route, navigate] as const;
}
