# MyGateway 测试脚本 v2（第 3 步：鉴权 + 配置）
# 用法：先启动服务 (go run main.go)，再执行 .\test.ps1
# 特点：端口和 API Key 从 config.json 读取，测试跟着配置走

$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'   # 关闭进度条刷屏

# ---- 读取配置 ----
$cfgObj = Get-Content -Raw -Encoding UTF8 ./config.json | ConvertFrom-Json
$port = $cfgObj.port -replace ':', ''
$Base = "http://localhost:$port"
$ValidKey = $cfgObj.api_key

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

# 断言 HTTP 错误状态码的小工具：执行请求，预期它失败且状态码为 $ExpectCode
function Assert-HttpCode($Name, $ScriptBlock, $ExpectCode) {
    try {
        $null = & $ScriptBlock
        Check $Name $false "请求居然成功了（期望 $ExpectCode）"
    } catch {
        $code = $_.Exception.Response.StatusCode.value__
        Check $Name ($code -eq $ExpectCode) "实际: $code, 期望: $ExpectCode"
    }
}

Write-Host "`n===== MyGateway 测试开始 (目标: $Base) =====`n"

# ---- 用例 1: /health 无需鉴权，200 + status=ok ----
try {
    $r = Invoke-WebRequest -UseBasicParsing "$Base/health"
    Check "GET /health 状态码为 200" ($r.StatusCode -eq 200) "实际: $($r.StatusCode)"
    Check "GET /health 返回 status=ok" ($r.Content -match '"status"\s*:\s*"ok"') "实际: $($r.Content)"
} catch {
    Check "GET /health 可访问" $false "异常: $($_.Exception.Message)"
}

# ---- 用例 2: /hello 无需鉴权，200 ----
try {
    $r = Invoke-WebRequest -UseBasicParsing "$Base/hello"
    Check "GET /hello 包含欢迎语" ($r.Content -match "Hello from MyGateway") "实际: $($r.Content)"
} catch {
    Check "GET /hello 可访问" $false "异常: $($_.Exception.Message)"
}

# ---- 用例 3（新增）: 不带 Authorization 请求 /proxy → 401 ----
Assert-HttpCode "POST /proxy 无 Authorization 头返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/proxy" -Body '{}' -ContentType "application/json"
} 401

# ---- 用例 4（新增）: 带错误 key → 401 ----
Assert-HttpCode "POST /proxy 错误 key 返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/proxy" -Body '{}' -ContentType "application/json" `
        -Headers @{Authorization = "Bearer wrong-key-123"}
} 401

# ---- 用例 5（新增）: 格式不对的头（没有 Bearer 前缀）→ 401 ----
Assert-HttpCode "POST /proxy 无 Bearer 前缀返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/proxy" -Body '{}' -ContentType "application/json" `
        -Headers @{Authorization = $ValidKey}
} 401

# ---- 用例 6: 带合法 key → 200 + httpbin 回显 ----
try {
    $payload = '{"hello":"world","from":"MyGateway-test"}'
    $r = Invoke-WebRequest -UseBasicParsing -Method POST "$Base/proxy" -Body $payload `
        -ContentType "application/json" -Headers @{Authorization = "Bearer $ValidKey"}
    Check "POST /proxy 合法 key 返回 200" ($r.StatusCode -eq 200) "实际: $($r.StatusCode)"

    $obj = $r.Content | ConvertFrom-Json
    Check "上游回显了转发的 body" ($obj.json.hello -eq "world") `
        "回显 json 字段: $($obj.json | ConvertTo-Json -Compress)"
    Check "转发时带上了 Content-Type 头" ($obj.headers.'Content-Type' -eq "application/json") `
        "实际头: $($obj.headers.'Content-Type')"
} catch {
    Check "POST /proxy 合法 key 可访问" $false "异常: $($_.Exception.Message)"
}

# ---- 用例 7: 方法不匹配仍应 405 ----
Assert-HttpCode "GET /proxy 返回 405" {
    Invoke-WebRequest -UseBasicParsing "$Base/proxy" -Method GET
} 405

# ---- 用例 8（第4步新增）: 无 key 打 /stream → 401 ----
Assert-HttpCode "POST /stream 无 Authorization 头返回 401" {
    Invoke-WebRequest -UseBasicParsing -Method POST "$Base/stream" -Body '{}' -ContentType "application/json"
} 401

# ---- 用例 9（第4步新增）: 合法 key 打 /stream → SSE 内容逐条透传 ----
# 用 curl.exe -N 才能真正观察流式；检查 14 条 data 是否齐全、前缀没被叠加
$streamOut = & curl.exe -N -s -X POST "$Base/stream" -H "Authorization: Bearer $ValidKey" -d '{}'
$dataLines = @($streamOut | Where-Object { $_ -match '^data: \d+$' })
Check "SSE 转发收到 14 条 data 消息" ($dataLines.Count -eq 14) "实际: $($dataLines.Count) 条"
Check "SSE 格式干净(无 data: data: 叠加)" (-not ($streamOut -match 'data: data:')) "发现畸形行"

# ---- 汇总 ----
Write-Host "`n===== 测试结束: 通过 $Pass / 失败 $Fail =====" -ForegroundColor Cyan
if ($Fail -gt 0) { exit 1 } else { exit 0 }
