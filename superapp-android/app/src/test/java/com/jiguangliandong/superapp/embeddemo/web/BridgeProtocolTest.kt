package com.jiguangliandong.superapp.embeddemo.web

import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class BridgeProtocolTest {
    @Test
    fun contextCapabilitiesAreSerializedAsAJsonArray() {
        val response = BridgeProtocol.contextResponse(
            LaunchManifest(
                launchId = "launch_demo",
                clientId = CLIENT_ID,
                displayName = "Partner H5",
                origin = "https://partner.example",
                launchUrl = "https://partner.example/app?launch_id=launch_demo",
                capabilities = listOf("getAuthCode", "openPrivacySettings", "close"),
                expiresAt = Instant.parse("2030-01-01T00:00:00Z"),
                signedPayload = "header.payload.signature",
            ),
            locale = "zh-CN",
        )

        val reparsed = JSONObject(response.toString())
        assertEquals("1.0", reparsed.getString("sdk_version"))
        assertEquals(CLIENT_ID, reparsed.getString("client_id"))
        assertEquals("superapp", reparsed.getString("container"))
        assertEquals("zh-CN", reparsed.getString("locale"))
        assertEquals(
            listOf("getAuthCode", "openPrivacySettings", "close"),
            (0 until reparsed.getJSONArray("capabilities").length()).map {
                reparsed.getJSONArray("capabilities").getString(it)
            },
        )
    }

    @Test
    fun acceptsKnownScopesAndValidPkce() {
        val challenge = Base64.getUrlEncoder().withoutPadding().encodeToString(
            MessageDigest.getInstance("SHA-256").digest("verifier".toByteArray()),
        )
        val request = BridgeProtocol.parseAuthorizationRequest(
            JSONObject()
                .put("transaction_id", "ptx_demo")
                .put("client_id", CLIENT_ID)
                .put("scopes", JSONArray(listOf("auth_base", "profile.name")))
                .put("state", "abcdefghijklmnop")
                .put("code_challenge", challenge)
                .put("code_challenge_method", "S256"),
            CLIENT_ID,
        )
        assertEquals(listOf("auth_base", "profile.name"), request.scopes)
    }

    @Test
    fun rejectsUnknownScope() {
        val challenge = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { 1 })
        assertThrows(IllegalArgumentException::class.java) {
            BridgeProtocol.parseAuthorizationRequest(
                JSONObject()
                    .put("transaction_id", "ptx_demo")
                    .put("client_id", CLIENT_ID)
                    .put("scopes", JSONArray(listOf("auth_base", "admin.all")))
                    .put("state", "abcdefghijklmnop")
                    .put("code_challenge", challenge)
                    .put("code_challenge_method", "S256"),
                CLIENT_ID,
            )
        }
    }

    @Test
    fun rejectsClientIdFromH5WhenItDiffersFromManifest() {
        val challenge = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { 2 })
        assertThrows(IllegalArgumentException::class.java) {
            BridgeProtocol.parseAuthorizationRequest(
                JSONObject()
                    .put("transaction_id", "ptx_demo")
                    .put("client_id", "attacker")
                    .put("scopes", JSONArray(listOf("auth_base")))
                    .put("state", "abcdefghijklmnop")
                    .put("code_challenge", challenge)
                    .put("code_challenge_method", "S256"),
                CLIENT_ID,
            )
        }
    }

    private companion object {
        const val CLIENT_ID = "embcli_demo"
    }
}
