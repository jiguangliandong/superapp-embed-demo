export class PartnerSsoError extends Error {
  constructor(code, message, cause) {
    super(message, cause ? { cause } : undefined);
    this.name = "PartnerSsoError";
    this.code = code;
  }
}

export const requireText = (value, field) => {
  if (typeof value !== "string" || value.trim() === "") {
    throw new PartnerSsoError("invalid_argument", `${field} is required`);
  }
  return value;
};
