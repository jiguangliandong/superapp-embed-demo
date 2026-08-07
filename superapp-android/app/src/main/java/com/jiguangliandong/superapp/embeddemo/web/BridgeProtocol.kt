package com.jiguangliandong.superapp.embeddemo.web

import com.jiguangliandong.superapp.embeddemo.data.AuthorizationCodeRequest
import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject

object BridgeProtocol {
    val knownScopes = setOf(
        "auth_base",
        "profile.name",
        "profile.avatar",
        "contact.phone",
        "contact.email",
        "kyc.status",
    )

    fun contextResponse(manifest: LaunchManifest, locale: String): JSONObject =
        JSONObject()
            .put("sdk_version", "1.0")
            .put("client_id", manifest.clientId)
            .put("container", "superapp")
            // org.json does not consistently wrap Kotlin collections when running on
            // Android. The JS SDK contract requires this field to be a real JSON array.
            .put("capabilities", JSONArray(manifest.capabilities))
            .put("locale", locale)

    fun parseAuthorizationRequest(params: JSONObject, trustedClientId: String): AuthorizationCodeRequest {
        val transactionId = params.requireBoundedText("transaction_id", 256)
        val clientId = params.requireBoundedText("client_id", 128)
        require(clientId == trustedClientId) { "client_id mismatch" }
        val state = params.requireBoundedText("state", 512)
        require(state.length >= 16 && STATE.matches(state)) { "invalid state" }
        val challenge = params.requireBoundedText("code_challenge", 43)
        require(challenge.length == 43 && BASE64_URL.matches(challenge)) { "invalid code challenge" }
        require(Base64.getUrlDecoder().decode(challenge).size == 32) { "invalid code challenge" }
        val method = params.getString("code_challenge_method")
        require(method == "S256") { "only S256 is allowed" }

        val jsonScopes = params.getJSONArray("scopes")
        val scopes = (0 until jsonScopes.length()).map(jsonScopes::getString)
        require(scopes.isNotEmpty() && scopes.distinct() == scopes && scopes.all(knownScopes::contains)) {
            "invalid scopes"
        }
        return AuthorizationCodeRequest(
            transactionId = transactionId,
            clientId = clientId,
            scopes = scopes,
            state = state,
            codeChallenge = challenge,
            codeChallengeMethod = method,
        )
    }

    private fun JSONObject.requireBoundedText(name: String, maximum: Int): String {
        val value = getString(name)
        require(value.isNotEmpty() && value.length <= maximum) { "invalid $name" }
        return value
    }

    private val STATE = Regex("^[A-Za-z0-9_-]+$")
    private val BASE64_URL = Regex("^[A-Za-z0-9_-]{43}$")
}
