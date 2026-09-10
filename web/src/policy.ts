// SPDX-License-Identifier: AGPL-3.0-only
import type { Checks } from "./api";

export interface GoogleHostedDomains {
  restricted: boolean;
  domains: string;
  allowUnhosted: boolean;
}

export function readGoogleHostedDomains(
  value: string | string[] | undefined,
): GoogleHostedDomains {
  const options =
    value === undefined ? [] : Array.isArray(value) ? value : [value];
  if (!options.every((option) => typeof option === "string")) {
    throw new Error("Google hd must be a string or list of strings.");
  }
  return {
    restricted: value !== undefined,
    domains: options.filter((option) => option !== "").join("\n"),
    allowUnhosted: options.includes(""),
  };
}

// Clearing a structured field must remove just that field. A null provider
// explicitly clears its config; omitted providers remain unchanged at the API.
export function overlayClaims(
  checks: Checks,
  azure: string,
  google: GoogleHostedDomains,
): Checks {
  const result: Checks = structuredClone(checks);
  const update = (
    provider: string,
    key: string,
    value: string | string[] | undefined,
  ) => {
    const entry = { ...result[provider] };
    if (value === undefined) delete entry[key];
    else entry[key] = value;
    result[provider] = Object.keys(entry).length ? entry : null;
  };
  const tids = azure
    .split("\n")
    .map((t) => t.trim())
    .filter(Boolean);
  update("azure", "tid", tids.length ? tids : undefined);
  const domains = [
    ...new Set(
      google.domains
        .split("\n")
        .map((d) => d.trim())
        .filter(Boolean),
    ),
  ];
  if (google.allowUnhosted) domains.push("");
  update("google", "hd", google.restricted ? domains : undefined);
  return result;
}
