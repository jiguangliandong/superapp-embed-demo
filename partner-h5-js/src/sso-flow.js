import { PartnerSsoError } from "./sso-error.js";

// This is the protocol orchestration previously hidden inside the JavaScript SDK.
// The PKCE verifier intentionally stays in Partner Backend transaction storage.
export const authenticateWithoutSdk = async ({
  api,
  bridge,
  expectedClientId,
  scopes,
}) => {
  const transaction = await api.createTransaction(scopes);
  if (transaction.client_id !== expectedClientId) {
    throw new PartnerSsoError(
      "client_id_mismatch",
      "Partner transaction belongs to an unexpected Embed Client",
    );
  }

  const authorization = await bridge.getAuthCode(transaction);

  return api.completeLogin({
    transaction_id: transaction.transaction_id,
    code: authorization.code,
    state: authorization.state,
  });
};
