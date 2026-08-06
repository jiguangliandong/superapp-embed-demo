import { Router } from "express";

import { config } from "../config.js";
import {
  exchangeAuthorizationCode,
  fetchUserInfo,
  SuperappProtocolError,
} from "../services/superapp-client.js";
import {
  beginSsoTransaction,
  InvalidScopesError,
  InvalidSsoTransactionError,
  takeSsoTransaction,
} from "../services/sso-transactions.js";

const router = Router();

const isNonEmptyString = (value, maximumLength) =>
  typeof value === "string" &&
  value.length > 0 &&
  value.length <= maximumLength;

const sendLoginFailure = (response, status = 400) => {
  response.set({
    "Cache-Control": "no-store",
    Pragma: "no-cache",
  });

  response.status(status).json({
    code: "partner_login_failed",
    message: "Partner login could not be completed",
  });
};

const regenerateSession = (request) =>
  new Promise((resolve, reject) => {
    request.session.regenerate((error) => {
      if (error) {
        reject(error);
        return;
      }

      resolve();
    });
  });

const toPublicUser = (userinfo) => ({
  open_id: userinfo.openId,
  ...(userinfo.displayName !== undefined && {
    display_name: userinfo.displayName,
  }),
  ...(userinfo.avatarUrl !== undefined && {
    avatar_url: userinfo.avatarUrl,
  }),
  ...(userinfo.contactPhone !== undefined && {
    contact_phone: userinfo.contactPhone,
  }),
  ...(userinfo.contactEmail !== undefined && {
    contact_email: userinfo.contactEmail,
  }),
  ...(userinfo.kycStatus !== undefined && {
    kyc_status: userinfo.kycStatus,
  }),
});

router.post("/api/sso/bootstrap", (request, response) => {
  try {
    request.session.ssoInitialized = true;

    const bootstrap = beginSsoTransaction({
      browserSessionId: request.sessionID,
      requestedScopes: request.body?.scopes,
      allowedScopes: config.allowedScopes,
      clientId: config.clientId,
      ttlMs: config.transactionTtlMs,
    });

    response.set({
      "Cache-Control": "no-store",
      Pragma: "no-cache",
    });

    response.status(201).json(bootstrap);
  } catch (error) {
    if (error instanceof InvalidScopesError) {
      response.status(400).json({
        code: "invalid_scopes",
        message: error.message,
      });
      return;
    }

    throw error;
  }
});

router.post("/api/sso/complete", async (request, response) => {
  const transactionId = request.body?.transaction_id;
  const code = request.body?.code;
  const state = request.body?.state;

  if (
    !isNonEmptyString(transactionId, 256) ||
    !isNonEmptyString(code, 2048) ||
    !isNonEmptyString(state, 512)
  ) {
    sendLoginFailure(response);
    return;
  }

  try {
    const transaction = takeSsoTransaction({
      browserSessionId: request.sessionID,
      transactionId,
      state,
    });

    const token = await exchangeAuthorizationCode({
      code,
      codeVerifier: transaction.codeVerifier,
    });

    const userinfo = await fetchUserInfo(token.accessToken);

    if (userinfo.openId !== token.openId) {
      throw new Error("Token and UserInfo subject mismatch");
    }

    // Authentication succeeded. Rotate the Partner Session ID to prevent
    // session fixation before storing authenticated state.
    await regenerateSession(request);

    const now = Date.now();
    const publicUser = toPublicUser(userinfo);

    // express-session stores this object server-side. The browser cookie only
    // contains the signed Partner Session ID.
    request.session.partnerAuth = {
      authenticatedAt: new Date(now).toISOString(),
      clientId: config.clientId,
      openId: token.openId,
      user: publicUser,
      scope: token.scope,
      consentVersion: token.consentVersion,
      tokens: {
        accessToken: token.accessToken,
        accessTokenExpiresAt: new Date(
          now + token.accessTokenExpiresIn * 1000,
        ).toISOString(),
        refreshToken: token.refreshToken,
        refreshTokenExpiresAt: new Date(
          now + token.refreshTokenExpiresIn * 1000,
        ).toISOString(),
      },
    };

    response.set({
      "Cache-Control": "no-store",
      Pragma: "no-cache",
    });

    response.json({
      authenticated: true,
      user: publicUser,
      scope: token.scope,
      consent_version: token.consentVersion,
    });
  } catch (error) {
    if (error instanceof InvalidSsoTransactionError) {
      sendLoginFailure(response);
      return;
    }

    if (error instanceof SuperappProtocolError) {
      const status = [
        "invalid_grant",
        "invalid_scope",
      ].includes(error.code)
        ? 400
        : 502;

      sendLoginFailure(response, status);
      return;
    }

    console.error("Unexpected Partner SSO failure", {
      name: error?.name ?? "UnknownError",
    });

    sendLoginFailure(response, 500);
  }
});

export default router;