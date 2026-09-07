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

const expectedClientId = document
  .querySelector('meta[name="superapp-client-id"]')
  ?.getAttribute("content");

const elements = {
  loadingView: document.querySelector("#loading-view"),
  loadingMessage: document.querySelector("#loading-message"),
  loginView: document.querySelector("#login-view"),
  profileView: document.querySelector("#profile-view"),
  contextStatus: document.querySelector("#context-status"),
  launchStatus: document.querySelector("#launch-status"),
  ssoStatus: document.querySelector("#sso-status"),
  loginButton: document.querySelector("#login-button"),
  closeButtons: [...document.querySelectorAll("[data-close]")],
  errors: [...document.querySelectorAll("[data-error-detail]")],
  avatar: document.querySelector("#profile-avatar"),
  name: document.querySelector("#profile-name"),
  openId: document.querySelector("#profile-open-id"),
  phone: document.querySelector("#profile-phone"),
  email: document.querySelector("#profile-email"),
  kyc: document.querySelector("#profile-kyc"),
  scopes: document.querySelector("#profile-scopes"),
  consentVersion: document.querySelector("#profile-consent-version"),
  sessionExpires: document.querySelector("#profile-session-expires"),
  entryMode: document.querySelector("#profile-entry-mode"),
};

let sdk;
let capabilities = [];

const launchId = new URL(window.location.href).searchParams.get("launch_id");
elements.launchStatus.textContent = launchId ? "已检测到 launch_id" : "普通浏览器访问";
elements.launchStatus.classList.toggle("status-good", Boolean(launchId));

const stableErrorCode = (error) =>
  error instanceof SuperappEmbedError ? error.code : "unexpected_error";

const showError = (error) => {
  for (const element of elements.errors) {
    element.textContent = `操作未完成（${stableErrorCode(error)}）`;
    element.hidden = false;
  }
};

const clearError = () => {
  for (const element of elements.errors) {
    element.textContent = "";
    element.hidden = true;
  }
};

const updateControls = (busy = false) => {
  elements.loginButton.disabled = busy || !capabilities.includes("getAuthCode");
  for (const button of elements.closeButtons) {
    button.disabled = busy || !capabilities.includes("close");
  }
  elements.loginButton.textContent = busy ? "正在登录…" : "重新登录";
};

const displayValue = (value, fallback = "未返回或未设置") =>
  typeof value === "string" && value.trim() ? value : fallback;

const fallbackAvatar = (displayName) =>
  (displayName?.trim() || "U").slice(0, 1).toUpperCase();

const validateSession = (session) => {
  if (
    session?.authenticated !== true ||
    typeof session?.user !== "object" ||
    session.user === null ||
    typeof session.user.open_id !== "string" ||
    !Array.isArray(session.scope) ||
    typeof session.session_expires_at !== "string"
  ) {
    throw new Error("Invalid Partner session response");
  }
  return session;
};

const replacePath = (pathname) => {
  const url = new URL(window.location.href);
  url.pathname = pathname;
  window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
};

const formatDateTime = (value) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "未知"
    : new Intl.DateTimeFormat("zh-CN", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
};

const renderProfile = (session, entryMode) => {
  validateSession(session);
  const user = session.user;
  const displayName = displayValue(user.display_name, "SuperApp 用户");

  elements.name.textContent = displayName;
  elements.openId.textContent = displayValue(user.open_id);
  elements.phone.textContent = displayValue(user.contact_phone);
  elements.email.textContent = displayValue(user.contact_email);
  elements.kyc.textContent = displayValue(user.kyc_status);
  elements.scopes.textContent = Array.isArray(session.scope)
    ? session.scope.join(" · ")
    : "未返回";
  elements.consentVersion.textContent = String(session.consent_version ?? "未返回");
  elements.sessionExpires.textContent = formatDateTime(session.session_expires_at);
  elements.entryMode.textContent = entryMode;

  elements.avatar.replaceChildren();
  if (user.avatar_url) {
    const image = document.createElement("img");
    image.src = user.avatar_url;
    image.alt = `${displayName}的头像`;
    image.referrerPolicy = "no-referrer";
    elements.avatar.append(image);
  } else {
    elements.avatar.textContent = fallbackAvatar(displayName);
  }

  elements.loadingView.hidden = true;
  elements.loginView.hidden = true;
  elements.profileView.hidden = false;
  replacePath("/app/profile");
};

const showLogin = () => {
  elements.loadingView.hidden = true;
  elements.profileView.hidden = true;
  elements.loginView.hidden = false;
  replacePath("/app");
};

const showLoading = (message = "正在验证内部应用会话，请稍候…") => {
  elements.loginView.hidden = true;
  elements.profileView.hidden = true;
  elements.loadingView.hidden = false;
  elements.loadingMessage.textContent = message;
};

const fetchPartnerSession = async () => {
  const response = await fetch("/api/sso/session", {
    method: "GET",
    headers: { accept: "application/json" },
    credentials: "include",
  });
  const body = await response.json().catch(() => ({}));
  if (response.status === 401) return null;
  if (!response.ok) {
    throw new SuperappEmbedError(
      body.code ?? "partner_session_unavailable",
      body.message ?? `Partner Backend returned HTTP ${response.status}`,
    );
  }
  return validateSession(body);
};

const waitForPageLoad = () => new Promise((resolve) => {
  if (document.readyState === "complete") {
    resolve();
  } else {
    window.addEventListener("load", resolve, { once: true });
  }
});

const authenticate = async ({ automatic = false } = {}) => {
  const copy = automatic
    ? {
        loading: "正在建立内部应用会话…",
        pending: "正在通过 SuperApp 静默登录",
        successEntry: "自动完成内部 SSO 登录",
        failure: "内部应用登录失败",
      }
    : {
        loading: null,
        pending: "正在重新登录",
        successEntry: "重新发起内部 SSO 登录",
        failure: "登录失败",
      };

  clearError();
  if (copy.loading) {
    showLoading(copy.loading);
  } else {
    showLogin();
  }
  elements.ssoStatus.textContent = copy.pending;
  elements.ssoStatus.classList.remove("status-good");
  updateControls(true);

  try {
    const session = validateSession(await sdk.authenticate({
      bootstrapURL: "/api/sso/bootstrap",
      completeURL: "/api/sso/complete",
      scopes: requestedScopes,
    }));
    elements.ssoStatus.textContent = "SuperApp 登录成功";
    elements.ssoStatus.classList.add("status-good");
    renderProfile(session, copy.successEntry);
    return true;
  } catch (error) {
    if (automatic) showLogin();
    elements.ssoStatus.textContent = copy.failure;
    showError(error);
    return false;
  } finally {
    updateControls();
  }
};

const initialize = async () => {
  showLoading();
  try {
    if (!expectedClientId || expectedClientId === "__SUPERAPP_CLIENT_ID__") {
      throw new Error("Partner client ID was not injected by the backend");
    }
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
    elements.contextStatus.textContent = `SuperApp / ${context.locale || "未知语言"}`;
    elements.contextStatus.classList.add("status-good");
    elements.ssoStatus.textContent = "正在检查内部应用会话";
    updateControls();
  } catch (error) {
    showLogin();
    elements.contextStatus.textContent = "Bridge 不可用";
    elements.ssoStatus.textContent = "请在 SuperApp 中打开";
    showError(error);
    updateControls();
    return;
  }

  try {
    const session = await fetchPartnerSession();
    if (session) {
      renderProfile(session, "内部应用会话自动恢复");
      updateControls();
      return;
    }
  } catch (error) {
    showError(error);
  }

  // 内部应用的 AllowedScopes 已由管理员在准入时批准。缺少本应用会话时始终申请新的
  // Authorization Code；User Center 仍会校验 Client、Origin、Launch、Scope 和
  // Customer Session，但不会要求面向外部 Partner 的原生授权确认。
  await waitForPageLoad();
  if (await authenticate({ automatic: true })) return;
  updateControls();
};

elements.loginButton.addEventListener("click", async () => {
  await authenticate();
});

for (const button of elements.closeButtons) {
  button.addEventListener("click", async () => {
    clearError();
    try {
      await sdk.close();
    } catch (error) {
      showError(error);
    }
  });
}

initialize();
