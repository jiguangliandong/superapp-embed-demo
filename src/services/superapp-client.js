import { config } from "../config.js";
import { createClientAssertion } from "./client-assertion.js";

const CLIENT_ASSERTION_TYPE =
  "urn:ietf:params:oauth:client-assertion-type:jwt-bearer";

const MAX_RESPONSE_BYTES = 64 * 1024;
const REQUEST_TIMEOUT_MS = 10_000;

export class SuperappProtocolError extends Error {
  constructor(code, httpStatus) {
    super("SuperApp protocol request failed");
    this.name = "SuperappProtocolError";
    this.code = code;
    this.httpStatus = httpStatus;
  }
}

const readJsonResponse = async (response) => {
  const declaredLength = Number.parseInt(
    response.headers.get("content-length") ?? "0",
    10,
  );

  if (declaredLength > MAX_RESPONSE_BYTES) {
    throw new SuperappProtocolError(
      "response_too_large",
      response.status,
    );
  }

  const text = await response.text();

  if (Buffer.byteLength(text, "utf8") > MAX_RESPONSE_BYTES) {
    throw new SuperappProtocolError(
      "response_too_large",
      response.status,
    );
  }

  try {
    return JSON.parse(text);
  } catch {
    throw new SuperappProtocolError(
      "invalid_response",
      response.status,
    );
  }
};

const requestJson = async (url, options) => {
  let response;

  try {
    response = await fetch(url, {
      ...options,
      redirect: "error",
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch {
    throw new SuperappProtocolError("network_error", 502);
  }

  const contentType = response.headers.get("content-type") ?? "";

  if (!contentType.toLowerCase().startsWith("application/json")) {
    throw new SuperappProtocolError(
      "invalid_response",
      response.status,
    );
  }

  const body = await readJsonResponse(response);

  if (!response.ok) {
    const errorCode =
      typeof body?.error === "string"
        ? body.error
        : "server_error";

    throw new SuperappProtocolError(errorCode, response.status);
  }

  return body;
};

const requireString = (value, field) => {
  if (typeof value !== "string" || value.length === 0) {
    throw new SuperappProtocolError(
      `invalid_${field}`,
      502,
    );
  }

  return value;
};

const requirePositiveInteger = (value, field) => {
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new SuperappProtocolError(
      `invalid_${field}`,
      502,
    );
  }

  return value;
};

export const exchangeAuthorizationCode = async ({
  code,
  codeVerifier,
}) => {
  const clientAssertion = await createClientAssertion({
    clientId: config.clientId,
    keyId: config.keyId,
    audience: config.tokenEndpoint,
    privateKeyPath: config.privateKeyPath,
  });

  const form = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    code_verifier: codeVerifier,
    client_id: config.clientId,
    client_assertion_type: CLIENT_ASSERTION_TYPE,
    client_assertion: clientAssertion,
  });

  const body = await requestJson(config.tokenEndpoint, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: form,
  });

  const scope = requireString(body.scope, "scope")
    .split(" ")
    .filter(Boolean);

  if (
    body.token_type !== "Bearer" ||
    !scope.includes("auth_base")
  ) {
    throw new SuperappProtocolError(
      "invalid_token_response",
      502,
    );
  }

  return {
    tokenType: body.token_type,
    accessToken: requireString(
      body.access_token,
      "access_token",
    ),
    accessTokenExpiresIn: requirePositiveInteger(
      body.expires_in,
      "expires_in",
    ),
    refreshToken: requireString(
      body.refresh_token,
      "refresh_token",
    ),
    refreshTokenExpiresIn: requirePositiveInteger(
      body.refresh_token_expires_in,
      "refresh_token_expires_in",
    ),
    scope,
    openId: requireString(body.open_id, "open_id"),
    consentVersion: requirePositiveInteger(
      body.consent_version,
      "consent_version",
    ),
  };
};

export const fetchUserInfo = async (accessToken) => {
  const body = await requestJson(config.userinfoEndpoint, {
    method: "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${accessToken}`,
    },
  });

  return {
    openId: requireString(body.open_id, "open_id"),
    ...(typeof body.display_name === "string" && {
      displayName: body.display_name,
    }),
    ...(typeof body.avatar_url === "string" && {
      avatarUrl: body.avatar_url,
    }),
    ...(typeof body.contact_phone === "string" && {
      contactPhone: body.contact_phone,
    }),
    ...(typeof body.contact_email === "string" && {
      contactEmail: body.contact_email,
    }),
    ...(typeof body.kyc_status === "string" && {
      kycStatus: body.kyc_status,
    }),
  };
};