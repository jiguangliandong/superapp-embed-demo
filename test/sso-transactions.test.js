import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import test from "node:test";

import {
  beginSsoTransaction,
  InvalidSsoTransactionError,
  takeSsoTransaction,
} from "../src/services/sso-transactions.js";

const defaults = {
  requestedScopes: ["profile.name"],
  allowedScopes: ["auth_base", "profile.name"],
  clientId: "embcli_test",
  ttlMs: 300_000,
};

test("creates and consumes a transaction without exposing its verifier", () => {
  const bootstrap = beginSsoTransaction({
    ...defaults,
    browserSessionId: "session-success",
  });

  assert.equal(Object.hasOwn(bootstrap, "code_verifier"), false);
  assert.deepEqual(bootstrap.scopes, ["auth_base", "profile.name"]);

  const transaction = takeSsoTransaction({
    browserSessionId: "session-success",
    transactionId: bootstrap.transaction_id,
    state: bootstrap.state,
  });

  assert.equal(transaction.codeVerifier.length, 43);
  assert.equal(
    createHash("sha256")
      .update(transaction.codeVerifier)
      .digest("base64url"),
    bootstrap.code_challenge,
  );
});

test("rejects replay after a successful take", () => {
  const bootstrap = beginSsoTransaction({
    ...defaults,
    browserSessionId: "session-replay",
  });

  const input = {
    browserSessionId: "session-replay",
    transactionId: bootstrap.transaction_id,
    state: bootstrap.state,
  };

  takeSsoTransaction(input);
  assert.throws(() => takeSsoTransaction(input), InvalidSsoTransactionError);
});

test("an incorrect state consumes the transaction", () => {
  const bootstrap = beginSsoTransaction({
    ...defaults,
    browserSessionId: "session-state",
  });

  assert.throws(
    () => takeSsoTransaction({
      browserSessionId: "session-state",
      transactionId: bootstrap.transaction_id,
      state: "incorrect-state",
    }),
    InvalidSsoTransactionError,
  );

  assert.throws(
    () => takeSsoTransaction({
      browserSessionId: "session-state",
      transactionId: bootstrap.transaction_id,
      state: bootstrap.state,
    }),
    InvalidSsoTransactionError,
  );
});

test("a different browser session cannot take a transaction", () => {
  const bootstrap = beginSsoTransaction({
    ...defaults,
    browserSessionId: "session-owner",
  });

  assert.throws(
    () => takeSsoTransaction({
      browserSessionId: "session-attacker",
      transactionId: bootstrap.transaction_id,
      state: bootstrap.state,
    }),
    InvalidSsoTransactionError,
  );

  assert.throws(
    () => takeSsoTransaction({
      browserSessionId: "session-owner",
      transactionId: bootstrap.transaction_id,
      state: bootstrap.state,
    }),
    InvalidSsoTransactionError,
  );
});

test("rejects an expired transaction", () => {
  const bootstrap = beginSsoTransaction({
    ...defaults,
    browserSessionId: "session-expired",
    now: 1_000,
  });

  assert.throws(
    () => takeSsoTransaction({
      browserSessionId: "session-expired",
      transactionId: bootstrap.transaction_id,
      state: bootstrap.state,
      now: 301_001,
    }),
    InvalidSsoTransactionError,
  );
});
