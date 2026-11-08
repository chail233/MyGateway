# Burst-load helper: fire N parallel POSTs at /proxy, count status codes.
# Run from project root:  powershell -ExecutionPolicy Bypass -File test\burst.ps1 [-Key 191910] [-N 8]
# Rate limit expected: ~2/s refill, burst 3 -> some 429 will appear.
param(
    [string]$Key = "",
    [int]$N = 8
)

$cfgObj = Get-Content -Raw -Encoding UTF8 ./config.json | ConvertFrom-Json
$port = $cfgObj.port -replace ':', ''
$Base = "http://localhost:$port"
if ($Key -eq "") { $Key = $cfgObj.api_keys[0].key }
Write-Host "Target client key: $Key"
$jobs = 1..$N | ForEach-Object {
    Start-Job -ScriptBlock {
        param($b, $k)
        try {
            $r = Invoke-WebRequest -UseBasicParsing -Method POST "$b/proxy" -Body '{}' `
                -ContentType 'application/json' -Headers @{Authorization = "Bearer $k"} -TimeoutSec 30
            $r.StatusCode
        } catch {
            $code = $_.Exception.Response.StatusCode.value__
            if ($code) { $code } else { "ERR" }
        }
    } -ArgumentList $Base, $Key
}

$codes = $jobs | Wait-Job | Receive-Job
$jobs | Remove-Job

Write-Host "Raw codes: $($codes -join ', ')"
Write-Host ("200 => " + ($codes | Where-Object { $_ -eq 200 }).Count)
Write-Host ("429 => " + ($codes | Where-Object { $_ -eq 429 }).Count)
Write-Host ("Other => " + ($codes | Where-Object { $_ -ne 200 -and $_ -ne 429 }).Count)
