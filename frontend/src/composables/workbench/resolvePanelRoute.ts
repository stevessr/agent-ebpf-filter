import type {
  Router,
  RouteLocationRaw,
  RouteLocationNormalizedLoaded,
} from "vue-router";
import { isFeatureIncludedInFrontendBuild } from "../../config/featureFlags";
import type { FeatureID } from "../../types/feature";
/** RouterView does not run navigation/redirect guards for an explicit route prop. */
export function resolvePanelRoute(
  router: Router,
  to: RouteLocationRaw,
  current?: RouteLocationNormalizedLoaded,
): RouteLocationNormalizedLoaded {
  let route = router.resolve(to, current);
  for (let i = 0; i < 8; i++) {
    const redirect = route.matched.at(-1)?.redirect;
    if (!redirect) break;
    route = router.resolve(
      typeof redirect === "function"
        ? redirect(
            route as RouteLocationNormalizedLoaded,
            current || router.currentRoute.value,
          )
        : redirect,
    );
  }
  const feature = route.meta.feature as FeatureID | undefined;
  if (feature && !isFeatureIncludedInFrontendBuild(feature))
    route = router.resolve({
      name: "FeatureUnavailable",
      query: { feature, from: route.fullPath },
    });
  return route as RouteLocationNormalizedLoaded;
}
