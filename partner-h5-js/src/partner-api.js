import { PartnerSsoError, requireText } from "./sso-error.js";

const DEFAULT_TIMEOUT_MS = 10_000;

const isObject = (value) =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const requireScopes = (value) => {
  if (
    !Array.isArray(value) ||
    value.length === 0 ||
    value.some((scope) => typeof scope !== "string" || scope.trim() === "")
  ) {
    throw new PartnerSsoError("invalid_partner_response", "Invalid scopes");
  }
  return value;
};

export const validatePartnerSession = (session) => {
  if (
    !isObject(session) ||
    session.authenticated !== true ||
    !isObject(session.user) ||
    typeof session.user.open_id !== "string" ||
    session.user.open_id.trim() === "" ||
    typeof session.session_expires_at !== "string"
  ) {
    throw new PartnerSsoError(
      "invalid_partner_response",
      "Invalid Partner session response",
    );
  }
  requireScopes(session.scope);
  return session;
};

const validateBootstrap = (transaction) => {
  if (!isObject(transaction) || Object.hasOwn(transaction, "code_verifier")) {
    throw new PartnerSsoError(
      "invalid_partner_response",
      "Invalid Partner bootstrap response",
    );
  }
  requireText(transaction.transaction_id, "transaction_id");
  requireText(transaction.client_id, "client_id");
  requireText(transaction.state, "state");
  requireText(transaction.code_challenge, "code_challenge");
  requireScopes(transaction.scopes);
  return transaction;
};

export class PartnerApiClient {
  constructor({
    fetchImpl = globalThis.fetch?.bind(globalThis),
    timeoutMs = DEFAULT_TIMEOUT_MS,
    sessionURL = "/api/sso/session",
    bootstrapURL = "/api/sso/bootstrap",
    completeURL = "/api/sso/complete",
  } = {}) {
    if (typeof fetchImpl !== "function") {
      throw new PartnerSsoError("fetch_unavailable", "Fetch API is unavailable");
    }
    this.fetch = fetchImpl;
    this.timeoutMs = timeoutMs;
    this.sessionURL = sessionURL;
    this.bootstrapURL = bootstrapURL;
    this.completeURL = completeURL;
  }

  async getSession() {
    const { response, body } = await this.request(this.sessionURL, {
      method: "GET",
    });
    if (response.status === 401 && body.code === "partner_session_missing") {
      return null;
    }
    this.requireSuccess(response, body);
    return validatePartnerSession(body);
  }

  async createTransaction(scopes) {
    requireScopes(scopes);
    const { response, body } = await this.request(this.bootstrapURL, {
      method: "POST",
      body: JSON.stringify({ scopes }),
    });
    this.requireSuccess(response, body);
    return validateBootstrap(body);
  }

  async completeLogin({ transaction_id, code, state }) {
    requireText(transaction_id, "transaction_id");
    requireText(code, "code");
    requireText(state, "state");
    const { response, body } = await this.request(this.completeURL, {
      method: "POST",
      body: JSON.stringify({ transaction_id, code, state }),
    });
    this.requireSuccess(response, body);
    return validatePartnerSession(body);
  }

  async request(url, init) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const response = await this.fetch(url, {
        ...init,
        headers: {
          accept: "application/json",
          ...(init.body ? { "content-type": "application/json" } : {}),
          ...init.headers,
        },
        credentials: "include",
        signal: controller.signal,
      });
      const body = await response.json().catch(() => ({}));
      return { response, body };
    } catch (error) {
      const code = error?.name === "AbortError" ? "partner_request_timeout" : "network_error";
      throw new PartnerSsoError(code, "Partner Backend request failed", error);
    } finally {
      clearTimeout(timer);
    }
  }

  requireSuccess(response, body) {
    if (response.ok) return;
    throw new PartnerSsoError(
      typeof body?.code === "string" ? body.code : "partner_request_failed",
      typeof body?.message === "string"
        ? body.message
        : `Partner Backend returned HTTP ${response.status}`,
    );
  }
}
