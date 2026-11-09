# MyGateway 测试脚本 v3（第 7 步：OpenAI chat completion 代理）
# 用法（三个终端，均在项目根目录）：
#   1) go run ./cmd/mockstream
#   2) go run ./cmd/mygateway -config config.json
#   3) powershell -ExecutionPolicy Bypass -File .\test\test.ps1
# 注意：本文件含中文，必须保存为 UTF-8 with BOM，否则 PS 5.1 会报误导性语法错误
# 注意：测试用英文 prompt，避免 PS 5.1 对 -Body 的 ASCII 编码把中文变成 '?'

$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'

# ---- 读取配置 ----
$cfgObj = Get-Content -Raw -Encoding UTF8 ./config.json | ConvertFrom-Json

if (-not $cfgObj.api_keys -or $cfgObj.api_keys.Count -lt 1) {
    Write-Host "config.json 缺少 api_keys" -ForegroundColor Red; exit 1
}
if (-not $cfgObj.api_keys[0].upstream -or $cfgObj.api_keys[0].upstream.Count -lt 1) {
    Write-Host "config.json 的 api_keys[0].upstream 为空" -ForegroundColor Red; exit 1
}

$port     = $cfgObj.port -replace ':', ''
$Base     = "http://localhost:$port"
$MockBase = $cfgObj.api_keys[0].upstream[0].base_url.TrimEnd('/')
$ValidKey = $cfgObj.api_keys[0].key
$ValidKey2 = $null
if ($cfgObj.api_keys.Count -gt 1) { $ValidKey2 = $cfgObj.api_keys[1].key }

$script:Pass = 0
$script:Fail = 0

function Check($Name, $Condition, $Detail = "") {
    if ($Condition) {
        Write-Host "[PASS] $Name" -ForegroundColor Green
        $script:Pass++
    } else {
        Write-Host "[FAIL] $Name  $Detail" -ForegroundColor Red
        $script:Fail++
    }
}

# 断言 HTTP 错误状态码：执行请求，预期它失败且状态码为 $ExpectCode
function Assert-HttpCode($Name, $ScriptBlock, $ExpectCode) {
    try {
        $null = & $ScriptBlock
        Check $Name $false "请求居然成功了（期望 $ExpectCode）"
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        Check $Name ($code -eq $ExpectCode) "实际: $code, 期望: $ExpectCode"
    }
}

# ---- 探活：两个服务都必须先起来，否则后面全是误导性 FAIL ----
Write-Host "`n===== 探活 ====="

try {
    $null = Invoke-WebRequest -UseBasicParsing -Method POST "$MockBase/echo" -Body '{}' `
        -ContentType 'application/json' -TimeoutSec 3
} catch {
    Write-Host "[ABORT] mock 上游不可达 ($MockBase)，请先运行: go run ./cmd/mockstream" -ForegroundColor Red
    exit 1
}
Write-Host "[ OK ] mock 上游在线 $MockBase" -ForegroundColor Green

try {
    $null = Invoke-WebRequest -UseBasicParsing "$Base/health" -TimeoutSec 3
} catch {
    Write-Host "[ABORT] 网关不可达 ($Base)，请先运行: go run ./cmd/mygateway -config config.json" -ForegroundColor Red
    exit 1
}
Write-Host "[ OK ] 网关在线 $Base`n" -ForegroundColor Green

Write-Host "===== MyGateway 测试开始 (目标: $Base) =====`n"

# ============================================================
# A. 基础路由
# ============================================================
try {
    $r = Invoke-WebRequest -UseBasicParsing "$Base/health"
    Check "GET /health 状态码为 200" ($r.StatusCode -eq 200) "实际: $($r.StatusCode)"
    Check "GET /health 返回 status=ok" ($r.Content -match '"status"\s*:\s*"ok"') "实际: $($r.Content)"
} catch {
    Check "GET /health 可访问" $false "异常: $($_.Exception.Message)"
}

try {
    $r = Invoke-WebRequest -UseBasicParsing "$Base/hello"
    Check "GET /hello 包含欢迎语" ($r.Content -match "Hello from MyGateway") "实际: $($r.Content)"
} catch {
    Check "GET /hello 可访问" $false "异常: $($_.Exception.Message)"
}

# ============================================================
# B. 鉴权：三种非法请求都必须在网关层被拦下
# ============================================================
Assert-HttpCode "POST /v1/chat/completions 无 Authorization 头返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
        -Body '{}' -ContentType "application/json"
} 401

Assert-HttpCode "POST /v1/chat/completions 错误 key 返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
        -Body '{}' -ContentType "application/json" -Headers @{Authorization = "Bearer wrong-key-123"}
} 401

Assert-HttpCode "POST /v1/chat/completions 无 Bearer 前缀返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
        -Body '{}' -ContentType "application/json" -Headers @{Authorization = $ValidKey}
} 401

# 方法不匹配：Go 1.22+ ServeMux 的方法匹配会返回 405
Assert-HttpCode "GET /v1/chat/completions 返回 405" {
    Invoke-WebRequest -UseBasicParsing "$Base/v1/chat/completions"
} 405

# ============================================================
# C. 非流式代理
# ============================================================
$payload = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello-from-test"}]}'

# PS 5.1 给 native exe 传参会剥掉内部双引号，所以 curl 用的 body 一律走临时文件
$jsonBodyFile = Join-Path $env:TEMP 'mygateway-test-body.json'
Set-Content -Path $jsonBodyFile -Value $payload -Encoding ASCII -NoNewline

try {
    $r = Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
        -Body $payload -ContentType "application/json" -Headers @{Authorization = "Bearer $ValidKey"}

    Check "非流式请求返回 200" ($r.StatusCode -eq 200) "实际: $($r.StatusCode)"
    Check "响应 Content-Type 为 application/json" ($r.Headers['Content-Type'] -match 'application/json') `
        "实际: $($r.Headers['Content-Type'])"

    $obj = $r.Content | ConvertFrom-Json
    Check "响应 object 为 chat.completion" ($obj.object -eq 'chat.completion') "实际: $($obj.object)"
    Check "上游收到并回显了 prompt" ($obj.choices[0].message.content -match 'hello-from-test') `
        "实际 content: $($obj.choices[0].message.content)"
    Check "响应包含 usage 统计" ($null -ne $obj.usage.total_tokens) `
        "实际 usage: $($obj.usage | ConvertTo-Json -Compress)"
    Check "finish_reason 为 stop" ($obj.choices[0].finish_reason -eq 'stop') `
        "实际: $($obj.choices[0].finish_reason)"
} catch {
    Check "非流式请求可访问" $false "异常: $($_.Exception.Message)"
}

# 上游错误状态码必须原样透传（mock 对空 messages 返回 400）
Assert-HttpCode "上游 400 被透传给客户端" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
        -Body '{"model":"gpt-4o-mini","messages":[]}' -ContentType "application/json" `
        -Headers @{Authorization = "Bearer $ValidKey"}
} 400

# ============================================================
# D. 密钥替换（本项目的安全核心）
# ============================================================
# D1: 直接用「网关 Key」打上游，必须被拒 —— 证明 mock 的校验是真的
Assert-HttpCode "上游拒绝网关 Key 直连（证明 mock 在严格校验上游 Key）" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$MockBase/v1/chat/completions" `
        -Body $payload -ContentType "application/json" -Headers @{Authorization = "Bearer $ValidKey"}
} 401

# D2: 经网关用同一个「网关 Key」却能成功 -> 唯一解释是网关把它换成了 sk-mock-*
if ($ValidKey2) {
    try {
        $r2 = Invoke-WebRequest -UseBasicParsing -Method POST "$Base/v1/chat/completions" `
            -Body $payload -ContentType "application/json" -Headers @{Authorization = "Bearer $ValidKey2"}
        Check "第二个客户($ValidKey2)也能成功，说明按客户取各自 upstream" ($r2.StatusCode -eq 200) `
            "实际: $($r2.StatusCode)"
    } catch {
        Check "第二个客户($ValidKey2)可访问" $false "异常: $($_.Exception.Message) (多半是它的 upstream.key 配错了)"
    }
}

# ============================================================
# E. SSE 流式代理
# ============================================================
$streamBodyFile = Join-Path $env:TEMP 'mygateway-test-stream.json'
$streamPayload = '{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"stream-sentinel"}]}'
Set-Content -Path $streamBodyFile -Value $streamPayload -Encoding ASCII -NoNewline

# E1: 内容完整性 —— mock 发 8 条 data（role + 5 片段 + stop + [DONE]）
$streamOut = & curl.exe -N -s --max-time 30 -X POST "$Base/v1/chat/completions" `
    -H "Authorization: Bearer $ValidKey" -H "Content-Type: application/json" -d "@$streamBodyFile"

$dataLines = @($streamOut | Where-Object { $_ -match '^data: ' })
Check "SSE 收到 8 条 data 消息" ($dataLines.Count -eq 8) "实际: $($dataLines.Count) 条"
Check "流末尾有 data: [DONE]" ([bool]($streamOut -match '^data: \[DONE\]$')) "未找到 [DONE]"
Check "流中包含 role=assistant 的首块" ([bool]($streamOut -match '"role":"assistant"')) "未找到 role 块"
Check "流最后的 chunk 带 finish_reason=stop" ([bool]($streamOut -match '"finish_reason":"stop"')) "未找到 stop"
Check "SSE 格式干净(无 data: data: 叠加)" (-not ($streamOut -match 'data: data:')) "发现畸形行"

$badJson = 0
foreach ($line in $dataLines) {
    $text = $line.Substring(6).TrimEnd()
    if ($text -eq '[DONE]') { continue }
    try { $null = $text | ConvertFrom-Json } catch { $badJson++ }
}
Check "每个 data 行都是合法 JSON" ($badJson -eq 0) "坏行数: $badJson"

# E2: 真流式检测 —— mock 每块间隔 300ms，共 1.5s。
#     若网关攒完再吐，首字节时间会≈总耗时；若逐块转发，首字节应远小于总耗时。
$timing = & curl.exe -N -s -o NUL --max-time 30 `
    -w "%{time_starttransfer}|%{time_total}" -X POST "$Base/v1/chat/completions" `
    -H "Authorization: Bearer $ValidKey" -H "Content-Type: application/json" -d "@$streamBodyFile"
$timing = ($timing | Out-String).Trim()
$parts = $timing -split '\|'
$startTransfer = [double]::Parse($parts[0], [Globalization.CultureInfo]::InvariantCulture)
$totalTime     = [double]::Parse($parts[1], [Globalization.CultureInfo]::InvariantCulture)

Check "流式为逐块转发（首字节远早于总耗时）" `
    (($startTransfer -lt 0.8) -and ($totalTime -gt 1.0)) `
    "首字节: ${startTransfer}s, 总耗时: ${totalTime}s (期望 首字节<0.8 且 总耗时>1.0)"

# ============================================================
# F. gzip 协商：请求头透传的经典陷阱
# ============================================================
# 网关用 req.Header = r.Header.Clone() 会把客户端的 Accept-Encoding: gzip 透给上游，
# 上游据此压缩响应；但回写客户端时只 Set 了 Content-Type，丢掉了 Content-Encoding，
# 客户端拿到的是「没有任何说明的 gzip 二进制流」。
# 正确做法二选一：不透传 Accept-Encoding（让 Transport 自行协商并解压），
#                或者把 Content-Encoding 一并透传给客户端。
$gzFile = Join-Path $env:TEMP 'mygateway-test-gzip.bin'
& curl.exe -s --max-time 30 -o $gzFile -X POST "$Base/v1/chat/completions" `
    -H "Authorization: Bearer $ValidKey" -H "Content-Type: application/json" `
    -H "Accept-Encoding: gzip" -d "@$jsonBodyFile"

if (Test-Path $gzFile) {
    $bytes = [IO.File]::ReadAllBytes($gzFile)
    $isGzip = ($bytes.Length -ge 2 -and $bytes[0] -eq 0x1F -and $bytes[1] -eq 0x8B)
    Check "Accept-Encoding: gzip 时客户端拿到可读 JSON（不是裸 gzip 字节）" `
        (-not $isGzip) `
        "客户端收到 gzip 原始字节：网关透传了 Accept-Encoding 却丢了 Content-Encoding 头"
} else {
    Check "Accept-Encoding: gzip 请求有响应" $false "curl 未产出响应文件"
}

Remove-Item $jsonBodyFile, $streamBodyFile, $gzFile -ErrorAction SilentlyContinue

# ============================================================
# G. 回归：旧路由必须已下线
# ============================================================
Assert-HttpCode "旧路由 POST /proxy 已下线返回 404" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/proxy" -Body '{}' -ContentType "application/json"
} 404

Assert-HttpCode "旧路由 POST /stream 已下线返回 404" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/stream" -Body '{}' -ContentType "application/json"
} 404

# ---- 汇总 ----
Write-Host "`n===== 测试结束: 通过 $Pass / 失败 $Fail =====" -ForegroundColor Cyan
if ($Fail -gt 0) { exit 1 } else { exit 0 }
