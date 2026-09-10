// SPDX-License-Identifier: AGPL-3.0-only
import { test } from "node:test";
import assert from "node:assert/strict";
import { overlayClaims, readGoogleHostedDomains } from "./policy.ts";
test("clear removes edited claim and preserves unknown claims/providers", () => {
  const original = {
    azure: { tid: ["old"], custom: "retain" },
    google: { hd: "example.com", department: "research" },
    custom: { department: ["research"] },
  };
  assert.deepEqual(
    overlayClaims(original, "", readGoogleHostedDomains(undefined)),
    {
      azure: { custom: "retain" },
      google: { department: "research" },
      custom: { department: ["research"] },
    },
  );
  assert.deepEqual(original.azure.tid, ["old"]);
});
test("multiple hosted domains round trip without losing a blank option or other checks", () => {
  const hd = ["example.com", "second.example.org", ""];
  const fields = readGoogleHostedDomains(hd);
  assert.deepEqual(fields, {
    restricted: true,
    domains: "example.com\nsecond.example.org",
    allowUnhosted: true,
  });
  assert.deepEqual(
    overlayClaims({ google: { hd, custom: "keep" } }, "", fields).google,
    { hd, custom: "keep" },
  );
});
test("blank lines do not enable unhosted accounts", () => {
  assert.deepEqual(
    overlayClaims({}, " tenant-a \n\n tenant-b", {
      restricted: true,
      domains: " example.com \n\n second.example.org \nexample.com\n",
      allowUnhosted: false,
    }),
    {
      azure: { tid: ["tenant-a", "tenant-b"] },
      google: { hd: ["example.com", "second.example.org"] },
    },
  );
});
test("unrestricted, deny-all and registered-unhosted-only policies remain distinct", () => {
  const options: (string | string[] | undefined)[] = [
    undefined,
    [],
    [""],
    "",
    "example.com",
  ];
  for (const value of options) {
    const actual = overlayClaims({}, "", readGoogleHostedDomains(value)).google;
    assert.deepEqual(
      actual,
      value === undefined
        ? null
        : { hd: typeof value === "string" ? [value] : value },
    );
  }
});
test("disabling restriction removes hd while preserving other Google claims", () => {
  const fields = readGoogleHostedDomains(["example.com", ""]);
  fields.restricted = false;
  assert.deepEqual(
    overlayClaims(
      { google: { hd: ["example.com", ""], extra: "keep" } },
      "",
      fields,
    ).google,
    { extra: "keep" },
  );
});
