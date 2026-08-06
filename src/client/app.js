import {
  createSuperappEmbedSDK,
  SuperappEmbedError,
} from "@superapp/embed-sdk";

const requestedScopes = [
  "auth_base",
  "profile.name",
  "contact.phone",
  "contact.email",
  "kyc.status",
];

const expectedClientId = document.body.dataset.clientId;

const contextStatus = document.querySelector("#context-status");
const ssoStatus = document.querySelector("#sso-status");
const loginButton = document.querySelector("#login-button");
const privacyButton = document.querySelector("#privacy-button");
const closeButton = document.querySelector("#close-button");
const profile = document.querySelector("#profile");
const profileAvatar = document.querySelector("#profile-avatar");
const profileName = document.querySelector("#profile-name");
const profileDetails = document.querySelector("#profile-details");
const errorDetail = document.querySelector("#error-detail");

let sdk;
let capabilities = [];

const stableErrorCode = (error) =>
  error instanceof SuperappEmbedError
    ? error.code
    : "unexpected_error";

const setError = (error) => {
  errorDetail.textContent = `错误代码：${stableErrorCode(error)}`;
  errorDetail.hidden = false;
};

const clearError = () => {
  errorDetail.textContent = "";
  errorDetail.hidden = true;
};

const updateControls = (busy = false) => {
  loginButton.disabled =
    busy || !capabilities.includes("getAuthCode");
  privacyButton.disabled =
    busy || !capabilities.includes("openPrivacySettings");
  closeButton.disabled = busy || !capabilities.includes("close");
};

const fallbackAvatar = (displayName) => {
  const normalizedName = displayName?.trim() || "U";
  return normalizedName.slice(0, 1).toUpperCase();
};

const renderProfile = (session) => {
  const user = session.user;
  const displayName = user.display_name || "SuperApp 用户";

  profileName.textContent = displayName;
  profileDetails.textContent = JSON.stringify(
    {
      open_id: user.open_id,
      contact_phone: user.contact_phone,
      contact_email: user.contact_email,
      kyc_status: user.kyc_status,
      scope: session.scope,
      consent_version: session.consent_version,
    },
    null,
    2,
  );

  if (user.avatar_url) {
    profileAvatar.textContent = "";
    const image = document.createElement("img");
    image.src = user.avatar_url;
    image.alt = `${displayName}的头像`;
    image.referrerPolicy = "no-referrer";
    profileAvatar.append(image);
  } else {
    profileAvatar.textContent = fallbackAvatar(displayName);
  }

  profile.hidden = false;
};

const initialize = async () => {
  try {
    sdk = createSuperappEmbedSDK();
    const context = await sdk.getContext();

    if (
      context?.sdk_version !== "1.0" ||
      context?.container !== "superapp" ||
      context?.client_id !== expectedClientId ||
      !Array.isArray(context?.capabilities)
    ) {
      throw new Error("Invalid SuperApp context");
    }

    capabilities = context.capabilities;
    contextStatus.textContent = `SuperApp / ${context.locale}`;
    contextStatus.classList.add("success");
    updateControls();
  } catch (error) {
    contextStatus.textContent = "普通浏览器或 Bridge 不可用";
    ssoStatus.textContent = "只能在 SuperApp 中登录";
    setError(error);
    updateControls();
  }
};

loginButton.addEventListener("click", async () => {
  clearError();
  profile.hidden = true;
  ssoStatus.textContent = "正在请求授权…";
  ssoStatus.classList.remove("success");
  updateControls(true);

  try {
    const session = await sdk.authenticate({
      bootstrapURL: "/api/sso/bootstrap",
      completeURL: "/api/sso/complete",
      scopes: requestedScopes,
    });

    if (
      session?.authenticated !== true ||
      typeof session?.user !== "object" ||
      session.user === null
    ) {
      throw new Error("Invalid Partner session response");
    }

    ssoStatus.textContent = "登录成功";
    ssoStatus.classList.add("success");
    renderProfile(session);
  } catch (error) {
    ssoStatus.textContent = "登录失败";
    ssoStatus.classList.remove("success");
    setError(error);
  } finally {
    updateControls();
  }
});

privacyButton.addEventListener("click", async () => {
  clearError();

  try {
    await sdk.openPrivacySettings();
  } catch (error) {
    setError(error);
  }
});

closeButton.addEventListener("click", async () => {
  clearError();

  try {
    await sdk.close();
  } catch (error) {
    setError(error);
  }
});

initialize();
