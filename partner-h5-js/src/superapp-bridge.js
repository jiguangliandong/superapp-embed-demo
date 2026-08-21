import { PartnerSsoError, requireText } from "./sso-error.js";

const DEFAULT_TIMEOUT_MS = 15_000;

export class SuperappBridgeClient {
  constructor({
    bridge = globalThis.SuperappNativeBridge,
    timeoutMs = DEFAULT_TIMEOUT_MS,
  } = {}) {
    if (!bridge || typeof bridge.invoke !== "function") {
      throw new PartnerSsoError(
        "bridge_unavailable",
        "SuperApp Native Bridge is unavailable",
      );
    }
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
      throw new PartnerSsoError("invalid_argument", "timeoutMs must be positive");
    }
    this.bridge = bridge;
    this.timeoutMs = timeoutMs;
  }

  getContext() {
    return this.invoke("getContext", {});
  }

  async getAuthCode(transaction) {
    requireText(transaction?.transaction_id, "transaction_id");
    requireText(transaction?.client_id, "client_id");
    requireText(transaction?.state, "state");
    requireText(transaction?.code_challenge, "code_challenge");
    if (!Array.isArray(transaction?.scopes) || transaction.scopes.length === 0) {
      throw new PartnerSsoError("invalid_argument", "scopes are required");
    }

    const authorization = await this.invoke("getAuthCode", {
      transaction_id: transaction.transaction_id,
      client_id: transaction.client_id,
      scopes: transaction.scopes,
      state: transaction.state,
      code_challenge: transaction.code_challenge,
      code_challenge_method: "S256",
    });

    requireText(authorization?.code, "authorization code");
    requireText(authorization?.state, "authorization state");
    if (authorization.state !== transaction.state) {
      throw new PartnerSsoError(
        "state_mismatch",
        "Native authorization response state does not match",
      );
    }
    return authorization;
  }

  openPrivacySettings() {
    return this.invoke("openPrivacySettings", {});
  }

  close() {
    return this.invoke("close", {});
  }

  async invoke(method, params) {
    let timer;
    try {
      return await Promise.race([
        Promise.resolve(this.bridge.invoke(method, params)),
        new Promise((_, reject) => {
          timer = setTimeout(() => {
            reject(new PartnerSsoError(
              "bridge_timeout",
              `SuperApp Native Bridge method ${method} timed out`,
            ));
          }, this.timeoutMs);
        }),
      ]);
    } catch (error) {
      if (error instanceof PartnerSsoError) throw error;
      const code = typeof error?.code === "string" && error.code
        ? error.code
        : "bridge_error";
      throw new PartnerSsoError(
        code,
        `SuperApp Native Bridge method ${method} failed`,
        error,
      );
    } finally {
      clearTimeout(timer);
    }
  }
}
