import { randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";

import { importPKCS8, SignJWT } from "jose";

let signingKeyPromise;

const loadSigningKey = (privateKeyPath) => {
  signingKeyPromise ??= readFile(privateKeyPath, "utf8").then((pem) =>
    importPKCS8(pem, "ES256"),
  );

  return signingKeyPromise;
};

export const createClientAssertion = async ({
  clientId,
  keyId,
  audience,
  privateKeyPath,
}) => {
  const signingKey = await loadSigningKey(privateKeyPath);
  const issuedAt = Math.floor(Date.now() / 1000);

  return new SignJWT({})
    .setProtectedHeader({
      alg: "ES256",
      kid: keyId,
      typ: "JWT",
    })
    .setIssuer(clientId)
    .setSubject(clientId)
    .setAudience(audience)
    .setIssuedAt(issuedAt)
    .setExpirationTime(issuedAt + 120)
    .setJti(randomUUID())
    .sign(signingKey);
};