package com.jiguangliandong.superapp.embeddemo.data

import java.time.Instant

data class CustomerSession(
    val accessToken: String,
    val expiresAt: Instant,
)

data class OTPChallenge(
    val challengeId: String,
    val phoneMasked: String,
    val expiresAt: Instant,
)

data class LaunchManifest(
    val launchId: String,
    val clientId: String,
    val displayName: String,
    val origin: String,
    val launchUrl: String,
    val capabilities: List<String>,
    val expiresAt: Instant,
    val signedPayload: String,
)

data class AuthorizationCodeRequest(
    val transactionId: String,
    val clientId: String,
    val scopes: List<String>,
    val state: String,
    val codeChallenge: String,
    val codeChallengeMethod: String,
)

data class AuthorizationCode(
    val code: String,
    val state: String,
    val expiresAt: Instant,
)

data class EmbedConsent(
    val clientId: String,
    val clientName: String,
    val grantedScopes: List<String>,
    val consentVersion: Int,
    val grantedAt: Instant,
    val expiresAt: Instant?,
    val privacyPolicyUrl: String,
)

class ApiException(
    val status: Int,
    val code: String,
    override val message: String,
) : Exception(message)
