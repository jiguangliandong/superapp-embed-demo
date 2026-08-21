package com.jiguangliandong.superapp.embeddemo.data

import com.jiguangliandong.superapp.embeddemo.BuildConfig
import java.io.BufferedReader
import java.net.HttpURLConnection
import java.net.Proxy
import java.net.URI
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject

class SuperappApi(
    private val baseUrl: String = BuildConfig.SUPERAPP_BASE_URL,
    private val jwksUrl: String = BuildConfig.EMBED_JWKS_URL,
) {
    suspend fun login(
        identifier: String,
        password: String,
        deviceId: String,
        deviceName: String,
    ): CustomerSession {
        val body = JSONObject()
            .put("identifier", identifier)
            .put("password", password)
            .put("device_id", deviceId)
            .put("device_name", deviceName)
        val json = request("POST", "$baseUrl/api/customer/v1/auth/login", body = body)
        return CustomerSession(
            accessToken = json.requireText("access_token"),
            expiresAt = Instant.parse(json.requireText("expires_at")),
        )
    }

    suspend fun launchManifest(token: String, clientId: String): LaunchManifest {
        @Suppress("DEPRECATION")
        val encodedClientId = URLEncoder.encode(clientId, StandardCharsets.UTF_8.name())
        val json = request(
            "GET",
            "$baseUrl/api/customer/v1/embed/apps/$encodedClientId/launch-manifest",
            token,
        )
        return LaunchManifest(
            launchId = json.requireText("launch_id"),
            clientId = json.requireText("client_id"),
            displayName = json.requireText("display_name"),
            origin = json.requireText("origin"),
            launchUrl = json.requireText("launch_url"),
            capabilities = json.requireStringList("capabilities"),
            expiresAt = Instant.parse(json.requireText("expires_at")),
            signedPayload = json.requireText("signed_payload"),
        )
    }

    suspend fun jwks(forceRefresh: Boolean = false): JSONObject {
        // The demo keeps no disk cache. The flag documents the unknown-kid retry contract.
        @Suppress("UNUSED_VARIABLE") val refresh = forceRefresh
        return request("GET", jwksUrl)
    }

    suspend fun createAuthorizationCode(
        token: String,
        manifest: LaunchManifest,
        pageOrigin: String,
        request: AuthorizationCodeRequest,
        consentApproved: Boolean,
    ): AuthorizationCode {
        val body = JSONObject()
            .put("client_id", manifest.clientId)
            .put("launch_id", manifest.launchId)
            .put("origin", pageOrigin)
            .put("scopes", JSONArray(request.scopes))
            .put("code_challenge", request.codeChallenge)
            .put("code_challenge_method", "S256")
            .put("state", request.state)
            .put("consent_approved", consentApproved)
        val json = request(
            "POST",
            "$baseUrl/api/customer/v1/embed/authorization-codes",
            token,
            body,
        )
        return AuthorizationCode(
            code = json.requireText("code"),
            state = json.requireText("state"),
            expiresAt = Instant.parse(json.requireText("expires_at")),
        )
    }

    suspend fun listAuthorizations(token: String): List<EmbedConsent> {
        val json = request("GET", "$baseUrl/api/customer/v1/embed/authorizations", token)
        val items = json.getJSONArray("items")
        return (0 until items.length()).map { index ->
            val item = items.getJSONObject(index)
            EmbedConsent(
                clientId = item.requireText("client_id"),
                clientName = item.requireText("client_name"),
                grantedScopes = item.requireStringList("granted_scopes"),
                consentVersion = item.getInt("consent_version"),
                grantedAt = Instant.parse(item.requireText("granted_at")),
                expiresAt = item.optString("expires_at").takeIf(String::isNotBlank)?.let(Instant::parse),
                privacyPolicyUrl = item.requireText("privacy_policy_url"),
            )
        }
    }

    suspend fun revokeAuthorization(token: String, clientId: String) {
        @Suppress("DEPRECATION")
        val encodedClientId = URLEncoder.encode(clientId, StandardCharsets.UTF_8.name())
        request("DELETE", "$baseUrl/api/customer/v1/embed/authorizations/$encodedClientId", token)
    }

    private suspend fun request(
        method: String,
        url: String,
        token: String? = null,
        body: JSONObject? = null,
    ): JSONObject = withContext(Dispatchers.IO) {
        // SuperApp Backend is reached through adb reverse on localhost:8080.
        // Bypass the emulator's global proxy explicitly; the proxy is only needed
        // by WebView when it opens the public Partner H5/ngrok URL.
        val connection = URI(url).toURL().openConnection(Proxy.NO_PROXY) as HttpURLConnection
        try {
            connection.requestMethod = method
            connection.connectTimeout = 10_000
            connection.readTimeout = 10_000
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json, application/problem+json")
            connection.setRequestProperty("Accept-Language", "zh-CN")
            connection.setRequestProperty("User-Agent", "SuperappEmbedAndroidDemo/1.0")
            if (token != null) connection.setRequestProperty("Authorization", "Bearer $token")
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
                connection.outputStream.use { output ->
                    output.write(body.toString().toByteArray(StandardCharsets.UTF_8))
                }
            }

            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val text = stream?.bufferedReader()?.use(BufferedReader::readText).orEmpty()
            val json = runCatching { JSONObject(text.ifBlank { "{}" }) }.getOrElse { JSONObject() }
            if (status !in 200..299) {
                throw ApiException(
                    status = status,
                    code = json.optString("code", "http_error"),
                    message = json.optString("detail", json.optString("title", "请求失败（HTTP $status）")),
                )
            }
            json
        } finally {
            connection.disconnect()
        }
    }
}

private fun JSONObject.requireText(name: String): String =
    getString(name).takeIf(String::isNotBlank) ?: error("$name is empty")

private fun JSONObject.requireStringList(name: String): List<String> {
    val values = getJSONArray(name)
    return (0 until values.length()).map(values::getString)
}
