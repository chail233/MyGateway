# MyGateway

一个用于学习 Go 网络服务开发的**模型 API 网关**（入门项目）。

它在客户端与上游模型服务之间做三件事：**认人**（API Key 鉴权）、**管额度**（按客户限流）、**转话**（把请求转发给上游，支持普通响应与 SSE 流式响应）。

> 纯标准库实现，仅一个第三方依赖 `golang.org/x/time/rate`（令牌桶）。

## 功能

- **API Key 鉴权**：`Authorization: Bearer <key>`，多 Key 注册表，`crypto/subtle` 常量时间比较防计时侧信道
- **按客户限流**：每个 Key 独立令牌桶（`rps` 补充速率 + `burst` 突发容量），超限返回 `429` 并带 `Retry-After`
- **请求转发**：`/proxy` 透传请求体与上游状态码
- **SSE 流式转发**：`/stream` 逐块读取上游事件流并即时 `Flush`，实现"逐字输出"效果
- **配置驱动**：JSON 配置文件 + `-config` 启动参数；启动时校验配置（fail-fast）
- **本地 mock 上游**：不依赖任何外部服务即可跑通全部功能与测试

## 目录结构

```
MyGateway/
├── cmd/
│   ├── mygateway/          # 网关主程序
│   └── mockstream/         # 本地 mock 上游（/stream SSE + /echo 回声）
├── internal/
│   ├── config/             # 配置结构、加载与校验
│   ├── clients/            # 客户注册表元素（名称 + 独立限流器）
│   └── middleware/         # Chain 组合器 + 鉴权限流中间件
├── test/
│   ├── test.ps1            # 回归测试（13 个用例）
│   ├── burst.ps1           # 并发压测（可指定客户与并发数）
│   └── config.test.json    # 测试专用宽松配置（限流拉满）
├── config.example.json     # 配置示例
└── config.json             # 实际配置（已被 .gitignore 忽略）
```

## 快速开始

要求：Go 1.27+

```powershell
# 1. 准备配置
Copy-Item config.example.json config.json

# 2. 启动本地 mock 上游（终端 1，监听 :5001）
go run ./cmd/mockstream

# 3. 启动网关（终端 2，监听 :5000）
go run ./cmd/mygateway -config config.json
```

访问验证：

```powershell
curl.exe http://localhost:5000/health
# {"status":"ok"}

curl.exe -X POST http://localhost:5000/proxy -H "Authorization: Bearer 114514" `
  -H "Content-Type: application/json" -d '{"hello":"world"}'
# mock 上游会把请求体与请求头原样回显

curl.exe -N -X POST http://localhost:5000/stream -H "Authorization: Bearer 114514" -d '{}'
# 约每 0.5 秒输出一条 data: 1 ... data: 14（-N 关闭 curl 缓冲才能看出流式节奏）
```

## 接口

| 方法 | 路径      | 鉴权 | 限流 | 说明                         |
|------|-----------|:----:|:----:|------------------------------|
| GET  | `/health` | 否   | 否   | 健康检查                     |
| GET  | `/hello`  | 否   | 否   | 示例接口                     |
| POST | `/proxy`  | 是   | 是   | 普通转发，透传状态码与响应体 |
| POST | `/stream` | 是   | 是   | SSE 流式转发                 |

错误约定：缺少/错误 Key → `401`；超出限额 → `429`（含 `Retry-After: 1`）；上游不可达 → `502`。

## 配置字段

| 字段         | 类型     | 说明                                   |
|--------------|----------|----------------------------------------|
| `port`       | string   | 监听地址，如 `":5000"`                 |
| `base_url`   | string   | `/proxy` 的上游地址                    |
| `stream_url` | string   | `/stream` 的 SSE 上游地址              |
| `api_keys`   | array    | 客户列表，每项含 `key`/`name`/`rps`/`burst` |

`rps` 是令牌补充速率（每秒放行数），`burst` 是突发容量（桶大小）。启动时任一字段非法，网关会打印原因并直接退出。

## 测试

两个脚本都在**项目根目录**运行（脚本内相对路径依赖工作目录）：

```powershell
# 回归测试：用宽松配置启动网关后执行
go run ./cmd/mygateway -config test/config.test.json
powershell -ExecutionPolicy Bypass -File .\test\test.ps1

# 并发压测：用默认（严格）配置启动网关后执行
powershell -ExecutionPolicy Bypass -File .\test\burst.ps1 -Key 114514 -N 8
powershell -ExecutionPolicy Bypass -File .\test\burst.ps1 -Key 191910 -N 8
```

预期：`alice`（rps=2/burst=3）会拿到一部分 `429`；`bob`（rps=10/burst=20）在 8 并发下全部 `200` —— 限流按客户隔离。

> 提示：`.ps1` 文件含中文时需保存为 **UTF-8 with BOM**，否则 Windows PowerShell 5.1 会因编码问题报出误导性的语法错误。

## 设计要点

- **中间件链**：`Chain(handler, mw1, mw2...)` 按书写顺序组合，鉴权先于限流（先确认身份，再扣该身份的额度）
- **注册表运行期只读**：客户表在启动时构建完毕，之后只读，因此无需加锁
- **流式转发三要素**：设置 `text/event-stream` 头、逐行读写、每块 `Flush`
- **超时策略**：不使用 `Client.Timeout` 一刀切；改用 `Transport.ResponseHeaderTimeout` 限制"等待响应头"的时间，流建立后不被打断
- **测试上游本地化**：公共上游（如 httpbin）自身会限流并污染并发测试结果，故全部测试指向本地 mock

## 后续可做

- 访问日志（记录调用方、状态码、耗时；需 `context` 传值与 `ResponseWriter` 包装）
- 失败重试（仅对"尚未收到响应"的连接类错误，流式响应绝不重试）
- 多模型路由（解析请求体 `model` 字段选择上游，并区分网关对外的 Key 与上游密钥）
- 优雅关闭（`http.Server.Shutdown` + 信号处理）
