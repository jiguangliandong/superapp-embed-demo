package com.jiguangliandong.superapp.embeddemo.security

import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class LaunchManifestVerifierTest {
    private val now = Instant.parse("2026-08-07T08:00:00Z")
    private val keyPair = KeyPairGenerator.getInstance("EC").apply {
        initialize(ECGenParameterSpec("secp256r1"))
    }.generateKeyPair()

    @Test
    fun verifiesSignedManifestAndBoundFields() {
        val fixture = fixture()
        val result = verifier().verify(fixture.first, fixture.second)
        assertEquals("superapp-test-key", result.kid)
    }

    @Test
    fun rejectsOuterManifestTampering() {
        val fixture = fixture()
        val tampered = fixture.first.copy(launchUrl = "https://partner.example/attacker")
        assertThrows(IllegalArgumentException::class.java) {
            verifier().verify(tampered, fixture.second)
        }
    }

    @Test
    fun rejectsUnknownSigningKey() {
        val fixture = fixture()
        val emptyJwks = JSONObject().put("keys", JSONArray())
        assertThrows(IllegalArgumentException::class.java) {
            verifier().verify(fixture.first, emptyJwks)
        }
    }

    private fun fixture(): Pair<LaunchManifest, JSONObject> {
        val expires = now.plusSeconds(60)
        val capabilities = listOf("getAuthCode", "openPrivacySettings", "close")
        val header = JSONObject().put("alg", "ES256").put("kid", "superapp-test-key")
        val claims = JSONObject()
            .put("iss", ISSUER)
            .put("sub", "lnch_demo")
            .put("aud", JSONArray(listOf(CLIENT_ID)))
            .put("iat", now.epochSecond)
            .put("exp", expires.epochSecond)
            .put("client_id", CLIENT_ID)
            .put("display_name", "Partner Demo")
            .put("origin", "https://partner.example")
            .put("launch_url", "https://partner.example/app")
            .put("capabilities", JSONArray(capabilities))
        val encodedHeader = encode(header.toString().toByteArray())
        val encodedClaims = encode(claims.toString().toByteArray())
        val signer = Signature.getInstance("SHA256withECDSA")
        signer.initSign(keyPair.private)
        signer.update("$encodedHeader.$encodedClaims".toByteArray(Charsets.US_ASCII))
        val signature = derToRaw(signer.sign())
        val jwt = "$encodedHeader.$encodedClaims.${encode(signature)}"

        val publicKey = keyPair.public as ECPublicKey
        val jwk = JSONObject()
            .put("kty", "EC")
            .put("crv", "P-256")
            .put("use", "sig")
            .put("alg", "ES256")
            .put("kid", "superapp-test-key")
            .put("x", encode(coordinate(publicKey.w.affineX.toByteArray())))
            .put("y", encode(coordinate(publicKey.w.affineY.toByteArray())))
        return LaunchManifest(
            launchId = "lnch_demo",
            clientId = CLIENT_ID,
            displayName = "Partner Demo",
            origin = "https://partner.example",
            launchUrl = "https://partner.example/app",
            capabilities = capabilities,
            expiresAt = expires,
            signedPayload = jwt,
        ) to JSONObject().put("keys", JSONArray().put(jwk))
    }

    private fun verifier() = LaunchManifestVerifier(
        issuer = ISSUER,
        expectedClientId = CLIENT_ID,
        clock = Clock.fixed(now, ZoneOffset.UTC),
    )

    private fun encode(value: ByteArray): String = Base64.getUrlEncoder().withoutPadding().encodeToString(value)

    private fun coordinate(value: ByteArray): ByteArray = when {
        value.size == 32 -> value
        value.size == 33 && value[0].toInt() == 0 -> value.copyOfRange(1, 33)
        value.size < 32 -> ByteArray(32 - value.size) + value
        else -> error("invalid coordinate")
    }

    private fun derToRaw(der: ByteArray): ByteArray {
        var offset = 2
        check(der[0].toInt() == 0x30)
        check(der[offset++].toInt() == 0x02)
        val rLength = der[offset++].toInt()
        val r = der.copyOfRange(offset, offset + rLength)
        offset += rLength
        check(der[offset++].toInt() == 0x02)
        val sLength = der[offset++].toInt()
        val s = der.copyOfRange(offset, offset + sLength)
        fun normalize(value: ByteArray): ByteArray = value.dropWhile { it.toInt() == 0 }
            .toByteArray().let { ByteArray(32 - it.size) + it }
        return normalize(r) + normalize(s)
    }

    private companion object {
        const val ISSUER = "http://localhost:8080"
        const val CLIENT_ID = "embcli_demo"
    }
}
