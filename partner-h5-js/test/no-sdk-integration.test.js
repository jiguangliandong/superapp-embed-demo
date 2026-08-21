import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { PartnerApiClient } from "../src/partner-api.js";
import { authenticateWithoutSdk } from "../src/sso-flow.js";
import { PartnerSsoError } from "../src/sso-error.js";
import { SuperappBridgeClient } from "../src/superapp-bridge.js";

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const sessionResponse = () => ({
  authenticated: true,
  user: { open_id: "open_demo", display_name: "Demo Customer" },
  scope: ["auth_base", "profile.name"],
  consent_version: 1,
  session_expires_at: "2026-08-21T12:00:00Z",
});

test("has no SuperApp JavaScript SDK dependency", async () => {
  const packageJSON = JSON.parse(
    await readFile(resolve(projectRoot, "package.json"), "utf8"),
  );
  assert.equal(packageJSON.dependencies, undefined);
  assert.equal(packageJSON.devDependencies.esbuild, "0.28.1");

  const files = ["app.js", "partner-api.js", "superapp-bridge.js", "sso-flow.js"];
  for (const file of files) {
    const source = await readFile(resolve(projectRoot, "src", file), "utf8");
    assert.doesNotMatch(source, /@superapp\/embed-sdk/);
  }
});

test("initial page shows only a neutral session restoration state", async () => {
  const html = await readFile(resolve(projectRoot, "static", "app.html"), "utf8");

  assert.match(html, /<section id="loading-view" class="loading-card view"/);
  assert.match(html, /<section id="login-view" class="hero view" hidden>/);
  assert.match(html, /<section id="profile-view" class="profile-card view" hidden>/);
});

test("automatic SSO does not trust browser storage as authorization state", async () => {
  const source = await readFile(resolve(projectRoot, "src", "app.js"), "utf8");

  assert.doesNotMatch(source, /localStorage|sessionStorage/);
  assert.match(source, /authenticate\(\{ automatic: true \}\)/);
});

test("storage diagnostics stays isolated from the SSO authorization module", async () => {
  const appHTML = await readFile(resolve(projectRoot, "static", "app.html"), "utf8");
  const privacyHTML = await readFile(resolve(projectRoot, "static", "privacy.html"), "utf8");
  const diagnostics = await readFile(
    resolve(projectRoot, "src", "storage-diagnostics.js"),
    "utf8",
  );

  assert.match(appHTML, /data-storage-diagnostics/);
  assert.match(privacyHTML, /data-storage-diagnostics/);
  assert.match(diagnostics, /document\.cookie/);
  assert.match(diagnostics, /localStorage/);
  assert.match(diagnostics, /sessionStorage/);
  assert.match(diagnostics, /caches\.open/);
});

test("automatic SSO uses consent-neutral copy until SuperApp decides", async () => {
  const source = await readFile(resolve(projectRoot, "src", "app.js"), "utf8");

  assert.doesNotMatch(source, /授权仍然有效|免授权恢复/);
  assert.match(source, /正在检查 SuperApp 授权状态/);
  assert.match(source, /正在通过 SuperApp 登录/);
  assert.match(source, /自动发起 SSO 登录/);
  assert.match(source, /用户主动完成 SSO 登录/);
  assert.match(source, /Partner 会话自动恢复/);
});

test("no-SDK flow calls Partner APIs and Native Bridge without exposing verifier", async () => {
  const requests = [];
  const bridgeCalls = [];
  const api = new PartnerApiClient({
    fetchImpl: async (url, init) => {
      requests.push({ url, init, body: JSON.parse(init.body) });
      if (url === "/api/sso/bootstrap") {
        return Response.json({
          transaction_id: "transaction-1",
          client_id: "embcli_demo",
          state: "state-with-enough-entropy",
          code_challenge: "challenge-without-verifier",
          scopes: ["auth_base", "profile.name"],
        }, { status: 201 });
      }
      return Response.json(sessionResponse());
    },
  });
  const bridge = new SuperappBridgeClient({
    bridge: {
      invoke: async (method, params) => {
        bridgeCalls.push({ method, params });
        return { code: "embcode_demo", state: params.state };
      },
    },
  });

  const result = await authenticateWithoutSdk({
    api,
    bridge,
    expectedClientId: "embcli_demo",
    scopes: ["auth_base", "profile.name"],
  });

  assert.equal(result.user.display_name, "Demo Customer");
  assert.equal(requests.length, 2);
  assert.equal(requests[0].url, "/api/sso/bootstrap");
  assert.equal(requests[1].url, "/api/sso/complete");
  assert.equal(requests[0].init.credentials, "include");
  assert.equal(requests[1].init.credentials, "include");
  assert.deepEqual(requests[1].body, {
    transaction_id: "transaction-1",
    code: "embcode_demo",
    state: "state-with-enough-entropy",
  });
  assert.deepEqual(bridgeCalls, [{
    method: "getAuthCode",
    params: {
      transaction_id: "transaction-1",
      client_id: "embcli_demo",
      scopes: ["auth_base", "profile.name"],
      state: "state-with-enough-entropy",
      code_challenge: "challenge-without-verifier",
      code_challenge_method: "S256",
    },
  }]);
  assert.equal("code_verifier" in bridgeCalls[0].params, false);
});

test("rejects a Native Bridge state mismatch before calling complete", async () => {
  const requests = [];
  const api = new PartnerApiClient({
    fetchImpl: async (url, init) => {
      requests.push(url);
      return Response.json({
        transaction_id: "transaction-1",
        client_id: "embcli_demo",
        state: "expected-state",
        code_challenge: "challenge",
        scopes: ["auth_base"],
      });
    },
  });
  const bridge = new SuperappBridgeClient({
    bridge: {
      invoke: async () => ({ code: "embcode_demo", state: "wrong-state" }),
    },
  });

  await assert.rejects(
    authenticateWithoutSdk({
      api,
      bridge,
      expectedClientId: "embcli_demo",
      scopes: ["auth_base"],
    }),
    (error) => error instanceof PartnerSsoError && error.code === "state_mismatch",
  );
  assert.deepEqual(requests, ["/api/sso/bootstrap"]);
});

test("rejects an unexpected client before asking SuperApp for a code", async () => {
  let bridgeCalled = false;
  const api = new PartnerApiClient({
    fetchImpl: async () => Response.json({
      transaction_id: "transaction-1",
      client_id: "embcli_attacker",
      state: "expected-state",
      code_challenge: "challenge",
      scopes: ["auth_base"],
    }),
  });
  const bridge = new SuperappBridgeClient({
    bridge: {
      invoke: async () => {
        bridgeCalled = true;
      },
    },
  });

  await assert.rejects(
    authenticateWithoutSdk({
      api,
      bridge,
      expectedClientId: "embcli_demo",
      scopes: ["auth_base"],
    }),
    (error) => error instanceof PartnerSsoError && error.code === "client_id_mismatch",
  );
  assert.equal(bridgeCalled, false);
});

test("rejects a bootstrap response that leaks the PKCE verifier", async () => {
  const api = new PartnerApiClient({
    fetchImpl: async () => Response.json({
      transaction_id: "transaction-1",
      client_id: "embcli_demo",
      state: "expected-state",
      code_challenge: "challenge",
      code_verifier: "must-never-enter-h5",
      scopes: ["auth_base"],
    }),
  });

  await assert.rejects(
    api.createTransaction(["auth_base"]),
    (error) =>
      error instanceof PartnerSsoError && error.code === "invalid_partner_response",
  );
});

test("treats only the stable missing-session response as logged out", async () => {
  const missingApi = new PartnerApiClient({
    fetchImpl: async () => Response.json(
      { code: "partner_session_missing", message: "Partner login is required" },
      { status: 401 },
    ),
  });
  assert.equal(await missingApi.getSession(), null);

  const failedApi = new PartnerApiClient({
    fetchImpl: async () => Response.json(
      { code: "partner_session_unavailable", message: "Try later" },
      { status: 502 },
    ),
  });
  await assert.rejects(
    failedApi.getSession(),
    (error) =>
      error instanceof PartnerSsoError && error.code === "partner_session_unavailable",
  );
});
