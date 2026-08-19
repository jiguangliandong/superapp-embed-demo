const markerKey = "superapp_isolation_probe";
const cacheName = "superapp-isolation-probe-v1";
const cacheRequestPath = "/__superapp_isolation_probe__";

function createMarker() {
  if (typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `probe-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function readCookie() {
  const prefix = `${encodeURIComponent(markerKey)}=`;
  const item = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  return item ? decodeURIComponent(item.slice(prefix.length)) : null;
}

async function readValues() {
  let cacheStorage = null;
  if ("caches" in window) {
    const cache = await caches.open(cacheName);
    const response = await cache.match(cacheRequestPath);
    cacheStorage = response ? await response.text() : null;
  }
  return {
    url: location.href,
    origin: location.origin,
    cookie: readCookie(),
    localStorage: localStorage.getItem(markerKey),
    sessionStorage: sessionStorage.getItem(markerKey),
    cacheStorage,
  };
}

async function writeValues(value) {
  document.cookie = `${encodeURIComponent(markerKey)}=${encodeURIComponent(value)}; Path=/; SameSite=Lax; Max-Age=86400`;
  localStorage.setItem(markerKey, value);
  sessionStorage.setItem(markerKey, value);
  if ("caches" in window) {
    const cache = await caches.open(cacheName);
    await cache.put(cacheRequestPath, new Response(value, {
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    }));
  }
}

async function clearValues() {
  document.cookie = `${encodeURIComponent(markerKey)}=; Path=/; SameSite=Lax; Max-Age=0`;
  localStorage.removeItem(markerKey);
  sessionStorage.removeItem(markerKey);
  if ("caches" in window) {
    await caches.delete(cacheName);
  }
}

function render(host) {
  host.innerHTML = `
    <h2>存储隔离测试</h2>
    <p>在容器 A 写入后，到容器 B 点击“读取”。同 Client ID 的 Cookie、LocalStorage、CacheStorage 应共享，但 SessionStorage 应隔离；不同 Client ID 应全部隔离。</p>
    <label>测试标记
      <input data-storage-value autocomplete="off" value="${createMarker()}" aria-label="存储测试标记">
    </label>
    <div class="storage-test-actions">
      <button class="primary" data-storage-write type="button">写入四种存储</button>
      <button class="secondary" data-storage-read type="button">读取当前值</button>
      <button class="secondary" data-storage-clear type="button">清除当前值</button>
    </div>
    <pre class="storage-test-output" data-storage-output aria-live="polite">尚未读取</pre>
  `;

  const input = host.querySelector("[data-storage-value]");
  const output = host.querySelector("[data-storage-output]");
  const run = async (operation) => {
    try {
      await operation();
      output.textContent = JSON.stringify(await readValues(), null, 2);
    } catch (error) {
      output.textContent = `操作失败: ${error instanceof Error ? error.message : String(error)}`;
    }
  };

  host.querySelector("[data-storage-write]").addEventListener("click", () => {
    void run(() => writeValues(input.value.trim() || createMarker()));
  });
  host.querySelector("[data-storage-read]").addEventListener("click", () => {
    void run(async () => {});
  });
  host.querySelector("[data-storage-clear]").addEventListener("click", () => {
    void run(clearValues);
  });
}

for (const host of document.querySelectorAll("[data-storage-diagnostics]")) {
  render(host);
}
