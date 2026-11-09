# MyGateway

一个用于学习 Go 网络服务开发的**模型 API 网关**（入门项目）。

它在客户端与上游模型服务之间做两件事：**认人**（API Key 鉴权）、**换钥转话**（把客户端 Key 换成真实上游密钥后转发请求，支持普通 JSON 与 SSE 流式响应）。

> 纯标准库实现，零第三方依赖。

## 功能

- **API Key 鉴权**：`Authorization: Bearer <key>`，启动时构建的多 Key 注册表；缺失/格式错误/未注册一律 `401`
- **客户 → 上游映射**：每个客户（`api_keys[i]`）拥有自己的上游列表，各自使用独立的 `base_url` 与上游密钥
- **密钥替换**：客户端只见网关 Key，真实上游 Key 只存在于配置文件中；转发时改写 `Authorization` 头
- **统一入口**：`POST /v1/chat/completions` 按上游响应的 `Content-Type` 自动分流——`application/json` 读全量后回写，`text/event-stream` 逐块转发并 `Flush`
- **响应头处理**：剥离 hop-by-hop 头（`Connection`、`Transfer-Encoding`、`Upgrade` 等，含 `Connection` 中显式点名的头），其余原样透传
- **gzip 正确性**：不把客户端的 `Accept-Encoding` 透传给上游，交给 `http.Transport` 自行协商并透明解压，避免客户端收到"没有 `Content-Encoding` 说明的 gzip 字节"
- **配置驱动**：JSON 配置 + `-config` 启动参数；启动时校验（fail-fast），非法直接退出
- **本地 mock 上游**：不依赖任何外部服务即可跑通全部功能与回归测试

## 目录结构

```
MyGateway/
├── cmd/
│   ├── mygateway/          # 网关主程序（路由 + completionHandler）
│   └── mockstream/         # 本地 mock 上游（:5001，/v1/chat/completions + /stream + /echo）
├── internal/
│   ├── clients/            # Client / APIKey / Upstream 结构定义
│   ├── config/             # 配置结构、加载与校验
│   ├── middleware/         # Chain 组合器 + Auth 鉴权中间件
│   ├── proxy/              # 上游请求构造、响应回写、SSE flushWriter、常量
│   └── utils/              # Authorization 头解析
├── test/
│   └── test.ps1            # 回归测试（A–G 七组，26 个断言）
├── config.example.json     # 配置示例（Key 与 mock 上游一一对应）
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

# 非流式
curl.exe -X POST http://localhost:5000/v1/chat/completions `
  -H "Authorization: Bearer 114514" -H "Content-Type: application/json" `
  -d '{\"model\":\"gpt-4o-mini\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}'
# {"id":"chatcmpl-mock-...","object":"chat.completion","choices":[...],"usage":{...}}

# 流式（-N 关闭 curl 缓冲，才能看出逐块节奏）
curl.exe -N -X POST http://localhost:5000/v1/chat/completions `
  -H "Authorization: Bearer 114514" -H "Content-Type: application/json" `
  -d '{\"model\":\"gpt-4o-mini\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}'
# data: {"object":"chat.completion.chunk",...}  每块间隔约 300ms
# data: [DONE]
```

> 注意：`-config` 默认值是相对路径 `./config.json`，**必须在项目根目录启动**，否则会报找不到配置文件；也可以传绝对路径。

## 接口

| 方法 | 路径                   | 鉴权 | 说明                                 |
|------|------------------------|:----:|--------------------------------------|
| GET  | `/health`              | 否   | 健康检查，返回 `{"status":"ok"}`     |
| GET  | `/hello`               | 否   | 示例接口                             |
| POST | `/v1/chat/completions` | 是   | OpenAI 兼容入口，自动分流 JSON / SSE |

错误约定：

- 缺少 `Authorization`、非 `Bearer ` 前缀、Key 未注册 → `401`
- 方法不匹配（如 `GET /v1/chat/completions`）→ `405`（Go 1.22+ ServeMux 方法匹配）
- 上游不可达（`gatewayClient.Do` 失败）→ `502`
- 上游非 2xx 状态码 → **原样透传**（如上游 400 就回 400）

## 配置字段

| 字段                             | 类型   | 说明                                                                |
|----------------------------------|--------|---------------------------------------------------------------------|
| `port`                           | string | 监听地址，如 `":5000"`                                              |
| `api_keys`                       | array  | 客户列表                                                            |
| `api_keys[].key`                 | string | 网关对外颁发的 Key（客户端使用）                                    |
| `api_keys[].name`                | string | 客户名，仅用于标识                                                  |
| `api_keys[].upstream`            | array  | 该客户可用的上游列表（当前取第 0 个）                               |
| `api_keys[].upstream[].base_url` | string | 上游根地址，如 `"http://127.0.0.1:5001"`，转发时拼接 `/v1/chat/completions` |
| `api_keys[].upstream[].key`      | string | 真实上游密钥，转发时替换 `Authorization`                            |

启动时 `port`、`api_keys[].key`、`api_keys[].name`、`upstream[].base_url`、`upstream[].key` 任一为空，网关会打印原因并直接退出。`upstream` 数组本身为空不会报错，但该客户的请求会在运行期得到 `500 No upstream configured`。

> 历史字段 `base_url` / `stream_url`（顶层，单上游时代的产物）**已废弃并从结构体中移除**，配置里出现会被 JSON 解码忽略。

`config.example.json` 中的两个上游密钥 `sk-mock-alice` / `sk-mock-bob` 与 mock 上游内置的校验表严格对应，改网关 Key 不影响它们，改这两个则 mock 会回 `401`。

## 测试

脚本在**项目根目录**运行（脚本内用相对路径读 `config.json`）：

```powershell
# 三个终端：mock 上游 → 网关 → 测试
go run ./cmd/mockstream
go run ./cmd/mygateway -config config.json
powershell -ExecutionPolicy Bypass -File .\test\test.ps1
```

覆盖的 7 组、26 个断言：

| 组 | 内容 |
|----|------|
| 探活 | mock 上游与网关都可达，否则直接 ABORT（避免后续全是误导性 FAIL） |
| A. 基础路由 | `/health` 状态码与响应体、`/hello` 欢迎语 |
| B. 鉴权 | 无头 / 错 Key / 无 `Bearer` 前缀 → `401`；`GET` 同一路径 → `405` |
| C. 非流式代理 | 200、`Content-Type`、`object`、prompt 回显、`usage`、`finish_reason`、上游 400 透传 |
| D. 密钥替换 | 直连 mock 用网关 Key 必须 `401`（证明 mock 真在校验）；第二个客户经网关仍 200（证明按客户取各自上游） |
| E. SSE 流式 | 收到 8 条 `data`、末尾 `[DONE]`、首块 `role`、末块 `finish_reason`、无 `data: data:` 叠加、每行合法 JSON；并用 `curl -w` 断言**首字节远早于总耗时**以证明真流式 |
| F. gzip 协商 | 带 `Accept-Encoding: gzip` 请求，客户端必须拿到可读 JSON 而非 gzip 原始字节 |
| G. 回归 | 旧路由 `/proxy`、`/stream` 已下线，返回 `404` |

> 提示：`.ps1` 文件含中文时需保存为 **UTF-8 with BOM**，否则 Windows PowerShell 5.1 会因编码问题报出误导性的语法错误。测试 Body 用英文 prompt，避免 PS 5.1 用 ASCII 编码 `-Body` 把中文变成 `?`。

## 设计要点

- **中间件链**：`Chain(handler, mw1, mw2...)` 逆序包裹，按书写顺序执行；新增中间件只需插入链中，handler 无需知情
- **单一 Handler 统一流式/非流式**：分流依据是上游响应的 `Content-Type` 而非请求体字段，因此网关不需要解析业务 JSON
- **注册表运行期只读**：客户表在启动时一次性构建，之后只读，因此无需加锁
- **上下文传值**：`Auth` 校验通过后把客户 Key 写入 `context`，handler 据此查注册表取上游，避免二次解析 `Authorization` 头
- **流式三要素**：设置 `text/event-stream`、逐块读写、每块 `Flush`（`flushWriter` 用 `http.NewResponseController` 包装，避免类型断言 `http.Flusher`）
- **超时策略**：不设 `Client.Timeout` 一刀切——它会连建立后的流一起掐断；上游半死不活的场景留给后续用 `context` 收口
- **测试上游本地化**：公共上游（如 httpbin）自身会限流并污染并发测试结果，故全部测试指向本地 mock

## 已知待改进

- `context` 中存放客户 Key 用的是裸 string key（`"client_key"`），建议改为自定义类型 + 取值辅助函数，让拼写错误在编译期暴露
- `WriteCompletionResponse` 在读上游 body 失败时，响应头里已经写入了上游头，错误响应会带着上游的 `Content-Type`；可先清空再报错
- 访问日志（调用方、状态码、耗时；需 `ResponseWriter` 包装）
- 失败重试（仅对"尚未收到响应"的连接类错误，流式响应绝不重试）
- 多上游路由与故障转移（目前只用 `upstream[0]`）
- 多模型路由（解析请求体 `model` 字段选择上游）
- 优雅关闭（`http.Server.Shutdown` + 信号处理）
