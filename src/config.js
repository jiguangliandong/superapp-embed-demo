const required = (name) => {
  const value = process.env[name]?.trim();

  if (!value) {
    throw new Error(`${name} is required`);
  }

  return value;
};

const parsePositiveInteger = (name, fallback) => {
  const rawValue = process.env[name]?.trim() || fallback;
  const value = Number.parseInt(rawValue, 10);

  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }

  return value;
};

const allowedScopes = required("PARTNER_ALLOWED_SCOPES")
  .split(",")
  .map((scope) => scope.trim())
  .filter(Boolean);

if (!allowedScopes.includes("auth_base")) {
  throw new Error("PARTNER_ALLOWED_SCOPES must include auth_base");
}

export const config = Object.freeze({
  superappBaseUrl: required("SUPERAPP_BASE_URL").replace(/\/$/, ""),
  clientId: required("SUPERAPP_CLIENT_ID"),
  keyId: required("SUPERAPP_KEY_ID"),
  privateKeyPath: required("SUPERAPP_PRIVATE_KEY_PATH"),
  allowedScopes: Object.freeze([...new Set(allowedScopes)]),
  transactionTtlMs:
    parsePositiveInteger("SSO_TRANSACTION_TTL_SECONDS", "300") * 1000,
  tokenEndpoint: required("SUPERAPP_TOKEN_ENDPOINT"),
  userinfoEndpoint: required("SUPERAPP_USERINFO_ENDPOINT"),
  revocationEndpoint: required("SUPERAPP_REVOCATION_ENDPOINT"),
});
