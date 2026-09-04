package com.jiguangliandong.superapp.embeddemo.web

import android.annotation.SuppressLint
import android.app.KeyguardManager
import android.content.Context
import android.graphics.Bitmap
import android.net.Uri
import android.net.http.SslError
import android.os.Handler
import android.os.Looper
import android.webkit.SslErrorHandler
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.webkit.JavaScriptReplyProxy
import androidx.webkit.WebMessageCompat
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import com.jiguangliandong.superapp.embeddemo.BuildConfig
import com.jiguangliandong.superapp.embeddemo.MainViewModel
import com.jiguangliandong.superapp.embeddemo.data.ApiException
import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import com.jiguangliandong.superapp.embeddemo.data.SuperappApi
import java.time.Instant
import java.util.Locale
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import org.json.JSONObject

@Composable
fun PartnerWebView(
    manifest: LaunchManifest,
    viewModel: MainViewModel,
    modifier: Modifier = Modifier,
) {
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val controller = remember(manifest.launchId) {
        PartnerWebViewController(
            manifest = manifest,
            tokenProvider = viewModel::customerToken,
            consent = { scopes -> viewModel.requestConsent(manifest, scopes) },
            openPrivacy = viewModel::openPrivacy,
            close = viewModel::closePartner,
            isForeground = { lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) },
        )
    }
    AndroidView(
        modifier = modifier,
        factory = { context -> controller.create(context) },
    )
    DisposableEffect(controller) {
        onDispose { controller.destroy() }
    }
}

private class PartnerWebViewController(
    private val manifest: LaunchManifest,
    private val tokenProvider: () -> String?,
    private val consent: suspend (List<String>) -> Boolean,
    private val openPrivacy: () -> Unit,
    private val close: () -> Unit,
    private val isForeground: () -> Boolean,
    private val api: SuperappApi = SuperappApi(),
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val authorizing = AtomicBoolean(false)
    private val mainHandler = Handler(Looper.getMainLooper())
    private var webView: WebView? = null
    private var trustedDocument = false
    private var navigating = true
    private var closed = false

    @SuppressLint("RequiresFeature", "SetJavaScriptEnabled")
    fun create(context: Context): WebView {
        check(WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) {
            "WebView does not support origin-scoped message listeners"
        }
        check(WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            "WebView does not support document-start scripts"
        }
        WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG)
        return WebView(context).also { view ->
            webView = view
            view.settings.apply {
                javaScriptEnabled = true
                domStorageEnabled = true
                allowFileAccess = false
                allowContentAccess = false
                javaScriptCanOpenWindowsAutomatically = false
                setSupportMultipleWindows(false)
                mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_NEVER_ALLOW
                cacheMode = android.webkit.WebSettings.LOAD_NO_CACHE
            }
            view.webViewClient = trustedClient()
            WebViewCompat.addDocumentStartJavaScript(view, BRIDGE_SCRIPT, setOf(manifest.origin))
            WebViewCompat.addWebMessageListener(
                view,
                "SuperappNativeHost",
                setOf(manifest.origin),
            ) { _, message, sourceOrigin, isMainFrame, replyProxy ->
                receive(context, message, sourceOrigin, isMainFrame, replyProxy)
            }
            view.loadUrl(targetUrl())
        }
    }

    fun destroy() {
        closed = true
        authorizing.set(false)
        (scope.coroutineContext[Job])?.cancel()
        webView?.apply {
            stopLoading()
            loadUrl("about:blank")
            clearHistory()
            removeAllViews()
            destroy()
        }
        webView = null
    }

    private fun receive(
        context: Context,
        message: WebMessageCompat,
        sourceOrigin: Uri,
        isMainFrame: Boolean,
        reply: JavaScriptReplyProxy,
    ) {
        val request = runCatching { JSONObject(message.data ?: "") }.getOrElse {
            reply.error("invalid_request", "Bridge request must be JSON")
            return
        }
        val requestId = request.optString("request_id")
        if (!REQUEST_ID.matches(requestId)) {
            reply.error("invalid_request", "Invalid request ID", requestId)
            return
        }
        if (!isMainFrame || sourceOrigin.toString().trimEnd('/') != manifest.origin ||
            !trustedDocument || closed
        ) {
            reply.error("untrusted_context", "Bridge call is not from the trusted main document", requestId)
            return
        }
        val method = request.optString("method")
        // The H5 SDK asks for its non-sensitive context while the initial document is
        // still loading. Authorization and all state-changing calls remain blocked until
        // onPageFinished, but getContext may safely rely on the main-frame source Origin.
        if (navigating && method != "getContext") {
            reply.error("navigation_in_progress", "Bridge call is unavailable during navigation", requestId)
            return
        }
        if (method !in manifest.capabilities && method != "getContext") {
            reply.error("capability_denied", "Capability is not available", requestId)
            return
        }
        val params = request.optJSONObject("params") ?: JSONObject()
        when (method) {
            "getContext" -> reply.success(
                requestId,
                BridgeProtocol.contextResponse(manifest, Locale.getDefault().toLanguageTag()),
            )
            "getAuthCode" -> authorize(context, params, requestId, reply)
            "openPrivacySettings" -> {
                reply.success(requestId, JSONObject())
                mainHandler.post(openPrivacy)
            }
            "close" -> {
                reply.success(requestId, JSONObject())
                mainHandler.post(close)
            }
            else -> reply.error("method_not_found", "Unknown Bridge method", requestId)
        }
    }

    private fun authorize(
        context: Context,
        params: JSONObject,
        requestId: String,
        reply: JavaScriptReplyProxy,
    ) {
        if (!authorizing.compareAndSet(false, true)) {
            reply.error("authorization_in_progress", "Another authorization is active", requestId)
            return
        }
        scope.launch {
            try {
                ensureRuntimeReady(context)
                val token = tokenProvider() ?: error("Customer session is unavailable")
                val request = BridgeProtocol.parseAuthorizationRequest(params, manifest.clientId)
                var code = try {
                    api.createAuthorizationCode(token, manifest, manifest.origin, request, false)
                } catch (error: ApiException) {
                    if (error.code != "embed.consent_required") throw error
                    if (!consent(request.scopes)) throw ConsentDenied()
                    ensureRuntimeReady(context)
                    api.createAuthorizationCode(token, manifest, manifest.origin, request, true)
                }
                ensureRuntimeReady(context)
                require(code.state == request.state) { "authorization state mismatch" }
                require(code.expiresAt.isAfter(Instant.now())) { "authorization code expired" }
                reply.success(
                    requestId,
                    JSONObject()
                        .put("code", code.code)
                        .put("state", code.state)
                        .put("expires_at", code.expiresAt.toString()),
                )
            } catch (_: ConsentDenied) {
                reply.error("consent_denied", "User declined authorization", requestId)
            } catch (error: ApiException) {
                reply.error(error.code, error.message, requestId)
            } catch (_: Throwable) {
                reply.error("authorization_failed", "Authorization could not be completed", requestId)
            } finally {
                authorizing.set(false)
            }
        }
    }

    private fun ensureRuntimeReady(context: Context) {
        val locked = (context.getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager).isDeviceLocked
        check(!closed && trustedDocument && !navigating && isForeground() && !locked) { "container unavailable" }
        check(manifest.expiresAt.isAfter(Instant.now())) { "manifest expired" }
    }

    private fun trustedClient() = object : WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
            if (!request.isForMainFrame) return false
            return originOf(request.url) != manifest.origin
        }

        override fun onPageStarted(view: WebView, url: String, favicon: Bitmap?) {
            navigating = true
            trustedDocument = originOf(Uri.parse(url)) == manifest.origin
        }

        override fun onPageFinished(view: WebView, url: String) {
            trustedDocument = originOf(Uri.parse(url)) == manifest.origin
            navigating = false
        }

        override fun onReceivedSslError(view: WebView, handler: SslErrorHandler, error: SslError) {
            trustedDocument = false
            handler.cancel()
        }
    }

    private fun targetUrl(): String = Uri.parse(manifest.launchUrl).buildUpon()
        .appendQueryParameter("launch_id", manifest.launchId)
        .build()
        .toString()

    private fun originOf(uri: Uri): String? {
        val scheme = uri.scheme ?: return null
        val host = uri.host ?: return null
        if (scheme != "https") return null
        val port = uri.port
        return if (port == -1 || port == 443) "$scheme://$host" else "$scheme://$host:$port"
    }

    @SuppressLint("RequiresFeature")
    private fun JavaScriptReplyProxy.success(requestId: String, result: JSONObject) {
        postMessage(JSONObject().put("request_id", requestId).put("ok", true).put("result", result).toString())
    }

    @SuppressLint("RequiresFeature")
    private fun JavaScriptReplyProxy.error(code: String, message: String, requestId: String = "") {
        postMessage(
            JSONObject()
                .put("request_id", requestId)
                .put("ok", false)
                .put("error", JSONObject().put("code", code).put("message", message))
                .toString(),
        )
    }

    private class ConsentDenied : Exception()

    companion object {
        private val REQUEST_ID = Regex("^[A-Za-z0-9_-]{16,128}$")
        private val BRIDGE_SCRIPT = """
            (() => {
              if (globalThis.SuperappNativeBridge) return;
              const pending = new Map();
              const randomId = () => {
                const bytes = crypto.getRandomValues(new Uint8Array(18));
                return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
              };
              SuperappNativeHost.onmessage = event => {
                let message;
                try { message = JSON.parse(event.data); } catch (_) { return; }
                const item = pending.get(message.request_id);
                if (!item) return;
                pending.delete(message.request_id);
                message.ok
                  ? item.resolve(message.result)
                  : item.reject(Object.assign(new Error(message.error?.message || 'Bridge failed'), {
                      code: message.error?.code || 'bridge_error'
                    }));
              };
              Object.defineProperty(globalThis, 'SuperappNativeBridge', {
                configurable: false,
                writable: false,
                value: Object.freeze({
                  invoke(method, params = {}) {
                    return new Promise((resolve, reject) => {
                      const request_id = randomId();
                      pending.set(request_id, { resolve, reject });
                      SuperappNativeHost.postMessage(JSON.stringify({ request_id, method, params }));
                      setTimeout(() => {
                        const item = pending.get(request_id);
                        if (!item) return;
                        pending.delete(request_id);
                        item.reject(Object.assign(new Error('Bridge timeout'), { code: 'bridge_timeout' }));
                      // getAuthCode may display native Consent; leave enough time for the user
                      // to review it. The H5 SDK timeout is intentionally slightly longer.
                      }, 120000);
                    });
                  }
                })
              });
            })();
        """.trimIndent()
    }
}
