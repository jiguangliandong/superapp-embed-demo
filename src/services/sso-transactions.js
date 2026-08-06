import {
  createHash,
  randomBytes,
  timingSafeEqual,
} from "node:crypto";

const MAX_ACTIVE_TRANSACTIONS_PER_SESSION = 5;
const transactions = new Map();

export class InvalidScopesError extends Error {
  constructor(message) {
    super(message);
    this.name = "InvalidScopesError";
  }
}

export class InvalidSsoTransactionError extends Error {
  constructor() {
    super("SSO transaction is invalid or expired");
    this.name = "InvalidSsoTransactionError";
    this.code = "invalid_sso_transaction";
  }
}

const randomBase64Url = (byteLength) =>
  randomBytes(byteLength).toString("base64url");

const normalizeScopes = (requestedScopes, allowedScopes) => {
  if (!Array.isArray(requestedScopes) || requestedScopes.length === 0) {
    throw new InvalidScopesError("scopes must be a non-empty array");
  }

  if (requestedScopes.some((scope) => typeof scope !== "string" || !scope)) {
    throw new InvalidScopesError("each scope must be a non-empty string");
  }

  if (new Set(requestedScopes).size !== requestedScopes.length) {
    throw new InvalidScopesError("scopes must not contain duplicates");
  }

  const allowed = new Set(allowedScopes);
  const unknownScope = requestedScopes.find((scope) => !allowed.has(scope));

  if (unknownScope) {
    throw new InvalidScopesError(`scope is not allowed: ${unknownScope}`);
  }

  return requestedScopes.includes("auth_base")
    ? [...requestedScopes]
    : ["auth_base", ...requestedScopes];
};

const pruneExpiredTransactions = (now) => {
  for (const [transactionId, transaction] of transactions) {
    if (transaction.expiresAt <= now) {
      transactions.delete(transactionId);
    }
  }
};

const pruneOldestTransactionsForSession = (browserSessionId) => {
  const activeForSession = [...transactions.entries()]
    .filter(([, transaction]) =>
      transaction.browserSessionId === browserSessionId)
    .sort(([, left], [, right]) => left.createdAt - right.createdAt);

  while (
    activeForSession.length >= MAX_ACTIVE_TRANSACTIONS_PER_SESSION
  ) {
    const [oldestTransactionId] = activeForSession.shift();
    transactions.delete(oldestTransactionId);
  }
};

const statesMatch = (expected, actual) => {
  const expectedBuffer = Buffer.from(expected, "utf8");
  const actualBuffer = Buffer.from(actual, "utf8");

  return (
    expectedBuffer.length === actualBuffer.length &&
    timingSafeEqual(expectedBuffer, actualBuffer)
  );
};

const isValidInput = (value, maximumLength) =>
  typeof value === "string" &&
  value.length > 0 &&
  value.length <= maximumLength;

export const beginSsoTransaction = ({
  browserSessionId,
  requestedScopes,
  allowedScopes,
  clientId,
  ttlMs,
  now = Date.now(),
}) => {
  if (!isValidInput(browserSessionId, 512)) {
    throw new InvalidSsoTransactionError();
  }

  const scopes = normalizeScopes(requestedScopes, allowedScopes);
  const transactionId = `ptx_${randomBase64Url(18)}`;
  const state = randomBase64Url(32);
  const codeVerifier = randomBase64Url(32);
  const codeChallenge = createHash("sha256")
    .update(codeVerifier)
    .digest("base64url");

  pruneExpiredTransactions(now);
  pruneOldestTransactionsForSession(browserSessionId);

  transactions.set(transactionId, {
    browserSessionId,
    clientId,
    state,
    codeVerifier,
    scopes,
    createdAt: now,
    expiresAt: now + ttlMs,
  });

  return {
    transaction_id: transactionId,
    client_id: clientId,
    state,
    code_challenge: codeChallenge,
    scopes,
  };
};

export const takeSsoTransaction = ({
  browserSessionId,
  transactionId,
  state,
  now = Date.now(),
}) => {
  if (
    !isValidInput(browserSessionId, 512) ||
    !isValidInput(transactionId, 256) ||
    !isValidInput(state, 512)
  ) {
    throw new InvalidSsoTransactionError();
  }

  const transaction = transactions.get(transactionId);

  // Delete before validation so invalid state/session attempts also consume the
  // transaction and concurrent calls cannot both obtain the verifier.
  transactions.delete(transactionId);

  if (
    !transaction ||
    transaction.expiresAt <= now ||
    transaction.browserSessionId !== browserSessionId ||
    !statesMatch(transaction.state, state)
  ) {
    throw new InvalidSsoTransactionError();
  }

  return {
    clientId: transaction.clientId,
    codeVerifier: transaction.codeVerifier,
    scopes: [...transaction.scopes],
  };
};
