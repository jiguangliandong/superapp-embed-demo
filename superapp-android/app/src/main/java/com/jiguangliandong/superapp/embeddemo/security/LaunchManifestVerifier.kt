package com.jiguangliandong.superapp.embeddemo.security

import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import java.math.BigInteger
import java.net.URI
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.util.Base64
import org.json.JSONObject

class LaunchManifestVerifier(
    private val issuer: String,
    private val expectedClientId: String,
    private val clock: Clock = Clock.systemUTC(),
    private val clockSkew: Duration = Duration.ofSeconds(60),
) {
    data class VerificationResult(val kid: String)

    fun verify(manifest: LaunchManifest, jwks: JSONObject): VerificationResult {
        require(manifest.clientId == expectedClientId) { "unexpected client_id" }
        validateLocation(manifest)
        val parts = manifest.signedPayload.split('.')
        require(parts.size == 3) { "signed_payload must be a compact JWS" }

        val header = decodeJson(parts[0])
        require(header.getString("alg") == "ES256") { "only ES256 is allowed" }
        val kid = header.getString("kid").also { require(it.isNotBlank()) { "kid is required" } }
        val key = findKey(jwks, kid)
        verifySignature(parts[0], parts[1], parts[2], key)

        val claims = decodeJson(parts[1])
        require(claims.getString("iss") == issuer) { "issuer mismatch" }
        require(claims.getString("sub") == manifest.launchId) { "subject mismatch" }
        require(singleAudience(claims) == manifest.clientId) { "audience mismatch" }
        require(claims.getString("client_id") == manifest.clientId) { "client_id claim mismatch" }
        require(claims.getString("display_name") == manifest.displayName) { "display_name claim mismatch" }
        require(claims.getString("origin") == manifest.origin) { "origin claim mismatch" }
        require(claims.getString("launch_url") == manifest.launchUrl) { "launch_url claim mismatch" }
        require(stringList(claims, "capabilities") == manifest.capabilities) { "capabilities claim mismatch" }

        val now = clock.instant()
        val issuedAt = Instant.ofEpochSecond(claims.getLong("iat"))
        val expiresAt = Instant.ofEpochSecond(claims.getLong("exp"))
        require(!issuedAt.isAfter(now.plus(clockSkew))) { "manifest issued in the future" }
        require(expiresAt.isAfter(now.minus(clockSkew))) { "manifest expired" }
        require(manifest.expiresAt.isAfter(now.minus(clockSkew))) { "outer manifest expired" }
        require(Duration.between(expiresAt, manifest.expiresAt).abs() < Duration.ofSeconds(1)) {
            "expires_at mismatch"
        }
        return VerificationResult(kid)
    }

    fun hasKid(jwks: JSONObject, kid: String): Boolean = runCatching {
        findKey(jwks, kid)
    }.isSuccess

    private fun findKey(jwks: JSONObject, kid: String): JSONObject {
        val keys = jwks.getJSONArray("keys")
        val matches = (0 until keys.length())
            .map(keys::getJSONObject)
            .filter { it.optString("kid") == kid }
        require(matches.size == 1) { "signing key not found or ambiguous" }
        return matches.single().also { key ->
            require(key.getString("kty") == "EC") { "key must be EC" }
            require(key.getString("crv") == "P-256") { "key must use P-256" }
            require(key.optString("use") == "sig") { "key use must be sig" }
            require(key.optString("alg") == "ES256") { "key algorithm mismatch" }
            require(!key.has("d")) { "private key material is forbidden" }
        }
    }

    private fun verifySignature(header: String, payload: String, encodedSignature: String, jwk: JSONObject) {
        val x = Base64.getUrlDecoder().decode(jwk.getString("x"))
        val y = Base64.getUrlDecoder().decode(jwk.getString("y"))
        require(x.size == 32 && y.size == 32) { "invalid P-256 coordinates" }
        val parameters = AlgorithmParameters.getInstance("EC").apply {
            init(ECGenParameterSpec("secp256r1"))
        }.getParameterSpec(ECParameterSpec::class.java)
        val publicKey = KeyFactory.getInstance("EC").generatePublic(
            ECPublicKeySpec(ECPoint(BigInteger(1, x), BigInteger(1, y)), parameters),
        )
        val rawSignature = Base64.getUrlDecoder().decode(encodedSignature)
        require(rawSignature.size == 64) { "invalid ES256 signature size" }
        val verifier = Signature.getInstance("SHA256withECDSA")
        verifier.initVerify(publicKey)
        verifier.update("$header.$payload".toByteArray(Charsets.US_ASCII))
        require(verifier.verify(rawToDer(rawSignature))) { "manifest signature is invalid" }
    }

    private fun validateLocation(manifest: LaunchManifest) {
        val origin = URI(manifest.origin)
        val launch = URI(manifest.launchUrl)
        require(origin.scheme == "https" && launch.scheme == "https") { "Partner URL must use HTTPS" }
        require(origin.userInfo == null && launch.userInfo == null) { "userinfo is forbidden" }
        require(origin.path.isNullOrEmpty() && origin.query == null && origin.fragment == null) { "origin must be exact" }
        require(normalizedOrigin(launch) == manifest.origin) { "launch URL origin mismatch" }
        require(manifest.capabilities.isNotEmpty() && manifest.capabilities.distinct() == manifest.capabilities) {
            "invalid capabilities"
        }
    }

    private fun normalizedOrigin(uri: URI): String {
        val defaultPort = (uri.scheme == "https" && uri.port == 443) || (uri.scheme == "http" && uri.port == 80)
        return buildString {
            append(uri.scheme)
            append("://")
            append(uri.host)
            if (uri.port != -1 && !defaultPort) append(":${uri.port}")
        }
    }

    private fun decodeJson(value: String): JSONObject = JSONObject(
        String(Base64.getUrlDecoder().decode(value), Charsets.UTF_8),
    )

    private fun singleAudience(claims: JSONObject): String {
        val audience = claims.get("aud")
        return when (audience) {
            is String -> audience
            else -> {
                val values = claims.getJSONArray("aud")
                require(values.length() == 1) { "audience must contain one value" }
                values.getString(0)
            }
        }
    }

    private fun stringList(json: JSONObject, name: String): List<String> {
        val values = json.getJSONArray(name)
        return (0 until values.length()).map(values::getString)
    }

    private fun rawToDer(raw: ByteArray): ByteArray {
        fun integer(bytes: ByteArray): ByteArray {
            val firstNonZero = bytes.indexOfFirst { it.toInt() != 0 }.let { if (it == -1) bytes.lastIndex else it }
            var value = bytes.copyOfRange(firstNonZero, bytes.size)
            if (value[0].toInt() and 0x80 != 0) value = byteArrayOf(0) + value
            return byteArrayOf(0x02, value.size.toByte()) + value
        }
        val r = integer(raw.copyOfRange(0, 32))
        val s = integer(raw.copyOfRange(32, 64))
        val sequence = r + s
        require(sequence.size < 128)
        return byteArrayOf(0x30, sequence.size.toByte()) + sequence
    }
}
