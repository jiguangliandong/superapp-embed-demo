import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  createSuperappEmbedSDK,
  SuperappEmbedError,
} from "@superapp/embed-sdk";

test("uses JavaScript SDK v0.0.2", async () => {
  const sdkEntryPath = fileURLToPath(import.meta.resolve("@superapp/embed-sdk"));
  const packagePath = resolve(dirname(sdkEntryPath), "..", "package.json");
  const packageJSON = JSON.parse(await readFile(packagePath, "utf8"));
  assert.equal(packageJSON.version, "0.0.2");
});

test("initial page shows only a neutral session restoration state", async () => {
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const html = await readFile(resolve(projectRoot, "static", "app.html"), "utf8");

  assert.match(html, /<section id="loading-view" class="loading-card view"/);
  assert.match(html, /<section id="login-view" class="hero view" hidden>/);
  assert.match(html, /<section id="profile-view" class="profile-card view" hidden>/);
});

test("automatic SSO does not trust browser storage as authorization state", async () => {
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const source = await readFile(resolve(projectRoot, "src", "app.js"), "utf8");

  assert.doesNotMatch(source, /localStorage|sessionStorage/);
  assert.match(source, /authenticate\(\{ automatic: true \}\)/);
});

test("storage diagnostics stays isolated from the SSO authorization module", async () => {
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
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
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const source = await readFile(resolve(projectRoot, "src", "app.js"), "utf8");

  assert.doesNotMatch(source, /授权仍然有效|免授权恢复/);
  assert.match(source, /正在检查 SuperApp 授权状态/);
  assert.match(source, /正在通过 SuperApp 登录/);
  assert.match(source, /自动发起 SSO 登录/);
  assert.match(source, /用户主动完成 SSO 登录/);
  assert.match(source, /Partner 会话自动恢复/);
});

test("authenticate connects bootstrap, Native Bridge and complete without exposing PKCE verifier", async () => {
  const requests = [];
  const bridgeCalls = [];
  const fetch = async (url, init) => {
    const body = JSON.parse(init.body);
    requests.push({ url, init, body });
    if (url === "/api/sso/bootstrap") {
      return Response.json({
        transaction_id: "transaction-1",
        client_id: "embcli_demo",
        state: "state-with-enough-entropy",
        code_challenge: "challenge-without-verifier",
        scopes: ["auth_base", "profile.name"],
      }, { status: 201 });
    }
    return Response.json({
      authenticated: true,
      user: { open_id: "open_demo", display_name: "Demo Customer" },
      scope: ["auth_base", "profile.name"],
      consent_version: 1,
    });
  };
  const bridge = {
    invoke: async (method, params) => {
      bridgeCalls.push({ method, params });
      return { code: "embcode_demo", state: params.state };
    },
  };

  const sdk = createSuperappEmbedSDK({ bridge, fetch });
  const result = await sdk.authenticate({
    bootstrapURL: "/api/sso/bootstrap",
    completeURL: "/api/sso/complete",
    scopes: ["auth_base", "profile.name"],
  });

  assert.equal(result.user.display_name, "Demo Customer");
  assert.equal(requests.length, 2);
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
  let requestCount = 0;
  const sdk = createSuperappEmbedSDK({
    bridge: {
      invoke: async () => ({ code: "embcode_demo", state: "wrong-state" }),
    },
    fetch: async () => {
      requestCount += 1;
      return Response.json({
        transaction_id: "transaction-1",
        client_id: "embcli_demo",
        state: "expected-state",
        code_challenge: "challenge",
        scopes: ["auth_base"],
      });
    },
  });

  await assert.rejects(
    sdk.authenticate({
      bootstrapURL: "/api/sso/bootstrap",
      completeURL: "/api/sso/complete",
    }),
    (error) => error instanceof SuperappEmbedError && error.code === "state_mismatch",
  );
  assert.equal(requestCount, 1);
});
