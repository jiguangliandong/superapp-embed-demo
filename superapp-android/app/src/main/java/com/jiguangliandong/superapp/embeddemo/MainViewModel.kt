package com.jiguangliandong.superapp.embeddemo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.jiguangliandong.superapp.embeddemo.data.ApiException
import com.jiguangliandong.superapp.embeddemo.data.CustomerSession
import com.jiguangliandong.superapp.embeddemo.data.EmbedConsent
import com.jiguangliandong.superapp.embeddemo.data.LaunchManifest
import com.jiguangliandong.superapp.embeddemo.data.SuperappApi
import com.jiguangliandong.superapp.embeddemo.security.LaunchManifestVerifier
import java.time.Instant
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

enum class AppScreen { LOGIN, HOME, PARTNER, PRIVACY }

data class AppUiState(
    val screen: AppScreen = AppScreen.LOGIN,
    val busy: Boolean = false,
    val message: String? = null,
    val manifest: LaunchManifest? = null,
    val consents: List<EmbedConsent> = emptyList(),
)

data class ConsentPrompt(
    val partnerName: String,
    val scopes: List<String>,
    val privacyUrl: String,
    internal val decision: CompletableDeferred<Boolean>,
)

class MainViewModel(
    private val api: SuperappApi = SuperappApi(),
    private val verifier: LaunchManifestVerifier = LaunchManifestVerifier(
        issuer = BuildConfig.EMBED_ISSUER,
        expectedClientId = BuildConfig.EMBED_CLIENT_ID,
    ),
) : ViewModel() {
    private val _uiState = MutableStateFlow(AppUiState())
    val uiState: StateFlow<AppUiState> = _uiState.asStateFlow()

    private val _consentPrompt = MutableStateFlow<ConsentPrompt?>(null)
    val consentPrompt: StateFlow<ConsentPrompt?> = _consentPrompt.asStateFlow()

    private var session: CustomerSession? = null
    private var otpChallengeId: String? = null
    private var otpPhoneNumber: String? = null

    fun requestOTP(phoneNumber: String, deviceId: String) {
        val phone = phoneNumber.trim()
        if (phone.isBlank()) {
            setMessage("请输入马来西亚手机号（不含国家区号）")
            return
        }
        viewModelScope.launch {
            setBusy(true)
            runCatching { api.createOTPChallenge(phone, deviceId) }
                .onSuccess { challenge ->
                    otpChallengeId = challenge.challengeId
                    otpPhoneNumber = phone
                    _uiState.value = _uiState.value.copy(
                        busy = false,
                        message = "验证码已发送至 ${challenge.phoneMasked}。local/dev 为该号码后六位。",
                    )
                }
                .onFailure { setFailure("发送验证码失败", it) }
            setBusy(false)
        }
    }

    fun login(phoneNumber: String, code: String, deviceId: String, deviceName: String) {
        val phone = phoneNumber.trim()
        val otp = code.trim()
        val challengeId = otpChallengeId
        if (phone.isBlank() || otp.length != 6 || otp.any { !it.isDigit() }) {
            setMessage("请输入手机号和六位数字验证码")
            return
        }
        if (challengeId == null || otpPhoneNumber != phone) {
            setMessage("请先为当前手机号获取验证码")
            return
        }
        viewModelScope.launch {
            setBusy(true)
            runCatching { api.verifyOTP(challengeId, phone, otp, deviceId, deviceName) }
                .onSuccess {
                    session = it
                    otpChallengeId = null
                    otpPhoneNumber = null
                    _uiState.value = AppUiState(screen = AppScreen.HOME)
                }
                .onFailure { setFailure("登录失败", it) }
            setBusy(false)
        }
    }

    fun openPartner() {
        val token = validToken() ?: return
        viewModelScope.launch {
            setBusy(true)
            runCatching {
                val manifest = api.launchManifest(token, BuildConfig.EMBED_CLIENT_ID)
                var jwks = api.jwks()
                try {
                    verifier.verify(manifest, jwks)
                } catch (_: IllegalArgumentException) {
                    jwks = api.jwks(forceRefresh = true)
                    verifier.verify(manifest, jwks)
                }
                manifest
            }.onSuccess { manifest ->
                _uiState.value = _uiState.value.copy(
                    screen = AppScreen.PARTNER,
                    manifest = manifest,
                    message = null,
                )
            }.onFailure { setFailure("无法安全打开 Partner H5", it) }
            setBusy(false)
        }
    }

    fun closePartner() {
        denyConsent()
        _uiState.value = _uiState.value.copy(
            screen = AppScreen.HOME,
            manifest = null,
            message = null,
        )
    }

    fun openPrivacy() {
        val token = validToken() ?: return
        viewModelScope.launch {
            setBusy(true)
            runCatching { api.listAuthorizations(token) }
                .onSuccess {
                    _uiState.value = _uiState.value.copy(
                        screen = AppScreen.PRIVACY,
                        consents = it,
                        message = null,
                    )
                }
                .onFailure { setFailure("无法读取授权记录", it) }
            setBusy(false)
        }
    }

    fun revoke(consent: EmbedConsent, onSuccess: () -> Unit = {}) {
        val token = validToken() ?: return
        viewModelScope.launch {
            setBusy(true)
            runCatching {
                api.revokeAuthorization(token, consent.clientId)
                api.listAuthorizations(token)
            }.onSuccess {
                _uiState.value = _uiState.value.copy(
                    screen = AppScreen.PRIVACY,
                    manifest = if (consent.clientId == BuildConfig.EMBED_CLIENT_ID) null else _uiState.value.manifest,
                    consents = it,
                    message = "授权已撤销",
                )
                onSuccess()
            }.onFailure { setFailure("撤销授权失败", it) }
            setBusy(false)
        }
    }

    fun backHome() {
        _uiState.value = _uiState.value.copy(screen = AppScreen.HOME, message = null)
    }

    fun logout() {
        denyConsent()
        session = null
        otpChallengeId = null
        otpPhoneNumber = null
        _uiState.value = AppUiState()
    }

    fun customerToken(): String? = validToken(showMessage = false)

    suspend fun requestConsent(manifest: LaunchManifest, scopes: List<String>): Boolean {
        check(_consentPrompt.value == null) { "another consent is active" }
        val decision = CompletableDeferred<Boolean>()
        _consentPrompt.value = ConsentPrompt(
            partnerName = manifest.displayName,
            scopes = scopes,
            privacyUrl = manifest.origin + "/privacy",
            decision = decision,
        )
        return try {
            decision.await()
        } finally {
            if (_consentPrompt.value?.decision === decision) _consentPrompt.value = null
        }
    }

    fun approveConsent() = resolveConsent(true)
    fun denyConsent() = resolveConsent(false)

    private fun resolveConsent(approved: Boolean) {
        _consentPrompt.value?.decision?.complete(approved)
        _consentPrompt.value = null
    }

    private fun validToken(showMessage: Boolean = true): String? {
        val current = session
        if (current == null || !current.expiresAt.isAfter(Instant.now())) {
            session = null
            _uiState.value = AppUiState(
                message = if (showMessage) "Customer 登录已失效，请重新登录" else null,
            )
            return null
        }
        return current.accessToken
    }

    private fun setBusy(value: Boolean) {
        _uiState.value = _uiState.value.copy(busy = value)
    }

    private fun setMessage(message: String) {
        _uiState.value = _uiState.value.copy(message = message)
    }

    private fun setFailure(prefix: String, error: Throwable) {
        val detail = when (error) {
            is ApiException -> "${error.message}（${error.code}）"
            is IllegalArgumentException -> "服务端启动信息未通过安全校验"
            else -> error.message ?: "未知错误"
        }
        _uiState.value = _uiState.value.copy(message = "$prefix：$detail")
    }
}
