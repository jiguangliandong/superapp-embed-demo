package com.jiguangliandong.superapp.embeddemo

import android.os.Bundle
import android.webkit.CookieManager
import android.webkit.WebStorage
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Apps
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.filled.OpenInBrowser
import androidx.compose.material.icons.filled.Security
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.net.toUri
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.jiguangliandong.superapp.embeddemo.data.EmbedConsent
import com.jiguangliandong.superapp.embeddemo.web.PartnerWebView
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.UUID

class MainActivity : ComponentActivity() {
    private val viewModel: MainViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val preferences = getSharedPreferences("demo-device", MODE_PRIVATE)
        val deviceId = preferences.getString("device-id", null) ?: UUID.randomUUID().toString().also {
            preferences.edit().putString("device-id", it).apply()
        }
        setContent {
            SuperappTheme {
                SuperappApp(
                    viewModel = viewModel,
                    deviceId = deviceId,
                    clearPartnerStorage = ::clearPartnerStorage,
                    openExternalUrl = { url ->
                        if (url.toUri().scheme == "https") {
                            startActivity(android.content.Intent(android.content.Intent.ACTION_VIEW, url.toUri()))
                        }
                    },
                )
            }
        }
    }

    private fun clearPartnerStorage() {
        CookieManager.getInstance().removeAllCookies(null)
        CookieManager.getInstance().flush()
        WebStorage.getInstance().deleteAllData()
    }
}

@Composable
private fun SuperappTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = androidx.compose.material3.lightColorScheme(
            primary = Color(0xFF3557D5),
            secondary = Color(0xFF6A5AE0),
            background = Color(0xFFF7F8FC),
            surface = Color.White,
            error = Color(0xFFBA1A1A),
        ),
        content = content,
    )
}

@Composable
private fun SuperappApp(
    viewModel: MainViewModel,
    deviceId: String,
    clearPartnerStorage: () -> Unit,
    openExternalUrl: (String) -> Unit,
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val consent by viewModel.consentPrompt.collectAsStateWithLifecycle()

    Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        when (state.screen) {
            AppScreen.LOGIN -> LoginScreen(
                busy = state.busy,
                message = state.message,
                onRequestOtp = { phone ->
                    viewModel.requestOTP(phone, deviceId)
                },
                onLogin = { phone, code ->
                    viewModel.login(phone, code, deviceId, "Android SSO Demo")
                },
            )
            AppScreen.HOME -> HomeScreen(
                busy = state.busy,
                message = state.message,
                onOpenPartner = viewModel::openPartner,
                onPrivacy = viewModel::openPrivacy,
                onLogout = {
                    clearPartnerStorage()
                    viewModel.logout()
                },
            )
            AppScreen.PARTNER -> PartnerScreen(state.manifest, viewModel)
            AppScreen.PRIVACY -> PrivacyScreen(
                state = state,
                onBack = viewModel::backHome,
                onRevoke = { item -> viewModel.revoke(item, clearPartnerStorage) },
                onOpenUrl = openExternalUrl,
            )
        }
        if (state.busy) {
            Box(
                Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.15f)),
                contentAlignment = Alignment.Center,
            ) {
                Card(shape = RoundedCornerShape(18.dp)) {
                    Row(
                        Modifier.padding(horizontal = 24.dp, vertical = 20.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 3.dp)
                        Spacer(Modifier.width(14.dp))
                        Text("正在连接 SuperApp…")
                    }
                }
            }
        }
    }

    consent?.let { prompt ->
        ConsentDialog(
            prompt = prompt,
            onApprove = viewModel::approveConsent,
            onDeny = viewModel::denyConsent,
            onPrivacy = { openExternalUrl(prompt.privacyUrl) },
        )
    }
}

@Composable
private fun LoginScreen(
    busy: Boolean,
    message: String?,
    onRequestOtp: (String) -> Unit,
    onLogin: (String, String) -> Unit,
) {
    var phoneNumber by rememberSaveable { mutableStateOf("") }
    var code by rememberSaveable { mutableStateOf("") }
    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Box(
            Modifier.size(64.dp).background(Color(0xFF3557D5), RoundedCornerShape(20.dp)),
            contentAlignment = Alignment.Center,
        ) { Icon(Icons.Default.Lock, null, tint = Color.White, modifier = Modifier.size(30.dp)) }
        Spacer(Modifier.height(24.dp))
        Text("登录 SuperApp", fontSize = 30.sp, fontWeight = FontWeight.Bold)
        Text(
            "使用 +60 手机号 OTP 登录用户中心，再从金刚位进入 Partner H5 完成端到端 SSO。",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 10.dp, bottom = 28.dp),
        )
        OutlinedTextField(
            value = phoneNumber,
            onValueChange = { phoneNumber = it },
            label = { Text("马来西亚手机号（不含 +60）") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(14.dp))
        OutlinedTextField(
            value = code,
            onValueChange = { code = it },
            label = { Text("六位验证码") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Message(message)
        OutlinedButton(
            onClick = { onRequestOtp(phoneNumber) },
            enabled = !busy,
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) { Text("发送验证码") }
        Spacer(Modifier.height(10.dp))
        Button(
            onClick = { onLogin(phoneNumber, code) },
            enabled = !busy,
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) { Text("登录") }
        Text(
            "local/dev 验证码是规范化 E.164 手机号的后六位。Customer Token 仅保存在本次 App 进程内，不会注入 H5。",
            fontSize = 12.sp,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 16.dp),
        )
    }
}

@Composable
private fun HomeScreen(
    busy: Boolean,
    message: String?,
    onOpenPartner: () -> Unit,
    onPrivacy: () -> Unit,
    onLogout: () -> Unit,
) {
    Scaffold(
        topBar = {
            DemoTopBar(
                title = "SuperApp",
                actions = {
                    IconButton(onClick = onPrivacy) { Icon(Icons.Default.Security, "隐私授权") }
                    IconButton(onClick = onLogout) { Icon(Icons.AutoMirrored.Filled.Logout, "退出登录") }
                },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(24.dp)) {
            Text("服务", fontSize = 24.sp, fontWeight = FontWeight.Bold)
            Text(
                "点击金刚位后，App 会从 SuperApp Backend 获取短期签名启动清单。",
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp, bottom = 22.dp),
            )
            Card(
                onClick = onOpenPartner,
                enabled = !busy,
                colors = CardDefaults.cardColors(containerColor = Color.White),
                elevation = CardDefaults.cardElevation(defaultElevation = 2.dp),
                shape = RoundedCornerShape(24.dp),
            ) {
                Row(
                    Modifier.fillMaxWidth().padding(20.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(
                        Modifier.size(58.dp).background(Color(0xFFEAEFFF), RoundedCornerShape(18.dp)),
                        contentAlignment = Alignment.Center,
                    ) { Icon(Icons.Default.Apps, null, tint = Color(0xFF3557D5), modifier = Modifier.size(30.dp)) }
                    Spacer(Modifier.width(16.dp))
                    Column(Modifier.weight(1f)) {
                        Text("Partner SSO Demo", fontWeight = FontWeight.SemiBold, fontSize = 18.sp)
                        Text("授权后在 H5 展示用户信息", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Icon(Icons.Default.OpenInBrowser, null, tint = Color(0xFF3557D5))
                }
            }
            Message(message)
        }
    }
}

@Composable
private fun PartnerScreen(manifest: com.jiguangliandong.superapp.embeddemo.data.LaunchManifest?, viewModel: MainViewModel) {
    if (manifest == null) {
        viewModel.closePartner()
        return
    }
    Scaffold(
        topBar = {
            DemoTopBar(
                title = manifest.displayName,
                navigation = {
                    IconButton(onClick = viewModel::closePartner) { Icon(Icons.Default.Close, "关闭 H5") }
                },
            )
        },
    ) { padding ->
        PartnerWebView(
            manifest = manifest,
            viewModel = viewModel,
            modifier = Modifier.fillMaxSize().padding(padding),
        )
    }
}

@Composable
private fun PrivacyScreen(
    state: AppUiState,
    onBack: () -> Unit,
    onRevoke: (EmbedConsent) -> Unit,
    onOpenUrl: (String) -> Unit,
) {
    var pendingRevoke by remember { mutableStateOf<EmbedConsent?>(null) }
    Scaffold(
        topBar = {
            DemoTopBar(
                title = "已授权应用",
                navigation = {
                    IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp),
        ) {
            if (state.consents.isEmpty()) {
                Text("暂无第三方授权", fontSize = 18.sp, fontWeight = FontWeight.SemiBold)
                Text("首次授权后会显示在这里。", color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            state.consents.forEach { item ->
                ConsentCard(item, { pendingRevoke = item }, { onOpenUrl(item.privacyPolicyUrl) })
                Spacer(Modifier.height(14.dp))
            }
            Message(state.message)
        }
    }
    pendingRevoke?.let { item ->
        AlertDialog(
            onDismissRequest = { pendingRevoke = null },
            title = { Text("撤销 ${item.clientName} 的授权？") },
            text = { Text("撤销后再次使用相关资料时，需要重新确认授权。") },
            confirmButton = {
                TextButton(onClick = { pendingRevoke = null; onRevoke(item) }) { Text("确认撤销") }
            },
            dismissButton = { TextButton(onClick = { pendingRevoke = null }) { Text("取消") } },
        )
    }
}

@Composable
private fun ConsentCard(item: EmbedConsent, onRevoke: () -> Unit, onPrivacy: () -> Unit) {
    val formatter = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")
        .withZone(ZoneId.systemDefault())
    Card(colors = CardDefaults.cardColors(containerColor = Color.White), shape = RoundedCornerShape(20.dp)) {
        Column(Modifier.fillMaxWidth().padding(18.dp)) {
            Text(item.clientName, fontSize = 18.sp, fontWeight = FontWeight.SemiBold)
            Text(
                "授权于 ${formatter.format(item.grantedAt)} · 条款 v${item.consentVersion}",
                fontSize = 12.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                item.grantedScopes.joinToString(" · ") { scopeLabel(it) },
                modifier = Modifier.padding(vertical = 14.dp),
            )
            Row {
                OutlinedButton(onClick = onPrivacy) { Text("隐私政策") }
                Spacer(Modifier.width(10.dp))
                TextButton(onClick = onRevoke) { Text("撤销授权", color = MaterialTheme.colorScheme.error) }
            }
        }
    }
}

@Composable
private fun ConsentDialog(
    prompt: ConsentPrompt,
    onApprove: () -> Unit,
    onDeny: () -> Unit,
    onPrivacy: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDeny,
        icon = { Icon(Icons.Default.Security, null) },
        title = { Text("允许 ${prompt.partnerName} 使用以下资料？") },
        text = {
            Column {
                Text("服务提供方：${prompt.partnerName}（已由 SuperApp 后台登记）")
                Spacer(Modifier.height(12.dp))
                prompt.scopes.forEach { Text("• ${scopeLabel(it)}：${scopePurpose(it)}") }
                Spacer(Modifier.height(12.dp))
                Text("你可以随时在“隐私授权”中撤销。", fontSize = 13.sp)
                TextButton(onClick = onPrivacy) { Text("查看 Partner 隐私政策") }
            }
        },
        confirmButton = { Button(onClick = onApprove) { Text("同意并继续") } },
        dismissButton = { TextButton(onClick = onDeny) { Text("暂不同意") } },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DemoTopBar(
    title: String,
    navigation: @Composable () -> Unit = {},
    actions: @Composable () -> Unit = {},
) {
    TopAppBar(
        title = { Text(title, fontWeight = FontWeight.SemiBold) },
        navigationIcon = navigation,
        actions = { actions() },
        colors = TopAppBarDefaults.topAppBarColors(containerColor = Color(0xFFF7F8FC)),
    )
}

@Composable
private fun Message(message: String?) {
    if (!message.isNullOrBlank()) {
        Text(
            message,
            color = MaterialTheme.colorScheme.error,
            fontSize = 13.sp,
            modifier = Modifier.padding(vertical = 14.dp),
        )
    } else {
        Spacer(Modifier.height(18.dp))
    }
}

private fun scopeLabel(scope: String): String = when (scope) {
    "auth_base" -> "基础身份"
    "profile.name" -> "昵称"
    "profile.avatar" -> "头像"
    "contact.phone" -> "已验证手机号"
    "contact.email" -> "已验证邮箱"
    "kyc.status" -> "实名认证状态"
    else -> scope
}

private fun scopePurpose(scope: String): String = when (scope) {
    "auth_base" -> "识别你在 Partner 中的账号"
    "profile.name" -> "在 Partner 页面展示称呼"
    "profile.avatar" -> "在 Partner 页面展示头像"
    "contact.phone" -> "展示已验证的联系方式"
    "contact.email" -> "展示已验证的联系方式"
    "kyc.status" -> "展示是否已完成实名认证"
    else -> "提供对应服务"
}
