# pi-go 选型静态评估

## 结论先行

这里的“pi-go”存在名称歧义，至少对应三个不同仓库：

- `github.com/guanshan/pi-go`：Pi coding agent 的 Go port；以下简称 **guanshan**。
- `github.com/dimetron/pi-go`：基于 Google ADK Go 的终端 coding agent；以下简称 **dimetron**。
- `github.com/OrdalieTech/pigo`：SDK-first 的 Pi port；以下简称 **pigo**。

本文件是基于公开 README、pkg.go.dev 文档和公开源码链接的**静态评估**，没有安装依赖、没有修改 `go.mod`、没有运行真实模型，也没有验证跨项目文件或协议的实际兼容性。GitHub raw 与 Go proxy 的 `go.mod` 地址在本次 `web_fetch` 中均未成功取得，因此 guanshan/pigo 的 Go 版本暂记为“未由本次来源确认”，不能臆测；dimetron README 的 Go 版本要求也只作为其 README 声称，不能视为本项目已验证。

**当前建议：不迁移。** 本项目已经实现并测试了与绘图比较工作流绑定的会话、恢复和接受态语义。外部库即使能减少通用 agent loop 或 JSONL 存储的维护，也不会自动减少本项目真正的兼容维护；直接替换会把格式、工具事件、模型协议和无密钥交付风险引入生产路径。

若未来确有引入需求，优先按“用户口中的 pi-go 最可能是 guanshan”做候选调查；从可嵌入粒度看，guanshan 的 public `packages/agent`/`packages/ai` 更适合先做局部 adapter 试验。若要采用更完整的 SDK，pigo 的 public `codingagent` 更直接，但侵入面更大。dimetron 的会话包在 `internal/session`，不适合外部嵌入。

## 当前项目的硬契约

代码范围：`go/session`、`go/runner`，并与 `go/drawing`、顶层比较 API 和 app 共同组成生产路径。当前 `go/go.mod` 为模块 `compare-figure`，声明 Go `1.25.0`，只依赖 `golang.org/x/image` 与 `gopkg.in/yaml.v3`。

### `go/session`

- `Client` 实现 OpenAI Chat Completions、OpenAI Responses、Anthropic 三种请求形状。
- 流式 SSE 与非流降级、请求重试、可取消退避保留在客户端路径。
- Responses 路径保留 reasoning encrypted content；消息中有 thinking/reasoning、签名和加密回放所需的内部字段。
- 支持文本与图片请求体；Transcript 会把 data URL 媒体写入 `media/`，以 SHA 派生文件名并在 JSONL 中使用受限 `file://media/...` 引用恢复。
- `Session` 自己负责工具循环、工具批次、轮次上限、token 预算估算、图像裁剪、工具结果裁剪和上下文压缩。
- `Transcript.Load` 处理 checkpoint、state、消息和未完成工具调用；进程中断时补写 interrupted tool result，避免恢复时重复假定工具已完成。
- 接受态由 submit 工具结果和保存的 state 恢复；已接受结果必须能够在没有模型密钥、没有再次调用模型的情况下继续交付。

### `go/runner`

- runner 负责配置/extends、输入指纹、动态门槛、复核、恢复、交付和锁。
- `run.json` 是当前 runner v3 格式，已有 accepted 输出时会在构造模型 client 之前返回。
- 接受态交付会重新编译已接受稿，但保留同次评分和路径语义；不同输入不能复用已有运行目录。
- drawing/comparison 的工具协议、图片/PDF 查看、编译、尺寸测量、比较和 submit 行为属于本项目生产边界，不是可被通用 agent 库替代的“普通工具”。

因此，任何替换都必须保留以下全部行为：

1. SSE 与三种模型协议适配。
2. thinking/signature/encrypted reasoning 的记录、回放和恢复。
3. 工具图像、媒体文件和安全路径引用。
4. 预算估算、工具结果压缩、上下文 checkpoint。
5. JSONL checkpoint、进程中断恢复和未完成工具处理。
6. 接受态无密钥恢复与交付。
7. drawing/comparison、尺寸与 submit/accept 语义。

不能因为某项目 README 写了“session persistence”“compaction”或“pi-compatible”，就视为满足上述契约。

## 候选一：guanshan/pi-go

来源：

- [模块与 README（pkg.go.dev）](https://pkg.go.dev/github.com/guanshan/pi-go)
- [public agent API](https://pkg.go.dev/github.com/guanshan/pi-go/packages/agent)
- [public harness session API](https://pkg.go.dev/github.com/guanshan/pi-go/packages/agent/harness/session)
- [许可证页面](https://pkg.go.dev/github.com/guanshan/pi-go@v0.78.0-go?tab=licenses)
- [GitHub 仓库](https://github.com/guanshan/pi-go)

### 可 import 的 public package

README 和 pkg.go.dev 列出的主要 public 包包括：

- `github.com/guanshan/pi-go/packages/ai`
- `github.com/guanshan/pi-go/packages/agent`
- `github.com/guanshan/pi-go/packages/agent/harness`
- `github.com/guanshan/pi-go/packages/agent/harness/session`
- `github.com/guanshan/pi-go/packages/coding-agent`
- `github.com/guanshan/pi-go/packages/tui`

其中 `packages/agent` 是可复用 loop kernel，公开 `Agent`、`RunAgentLoop`、事件、流、工具和消息类型；`packages/agent/harness/session` 是 public 的 session-tree 存储层，公开 `JSONLRepo`、`JSONLStorage`、`MemoryRepo`、`Session`、`Entry`、compaction、branch、custom entry 等 API。pkg.go.dev 页面显示其 `CurrentSessionVersion` 为 3。

这回答了“guanshan 哪些 packages public 能 import”：上述路径不在 `internal/`，从 Go 的包可见性规则上可以被本项目导入。是否稳定则是另一问题。

### 与当前项目的匹配

匹配点：

- Go 原生、包边界清楚，loop kernel 与持久化 harness 分层。
- public agent API 可以理论上承接当前 `Completer`/`Toolset` 之上的通用循环部分。
- public session API 有内存与 JSONL 两种后端，支持树、分支和 compaction。
- MIT 许可证，适合评估嵌入（仍需保留版权和许可证文本）。

风险和缺口：

- README 明确称 alpha，且上游兼容、磁盘格式和工具语义仍在推进，不是已经完成的 1:1 replacement。
- 其 session 是 Pi v3 tree（例如 id、parentId、timestamp、entry type），与本项目的 `t=meta/msg/usage/checkpoint/state` Transcript 不是同一格式。
- 其 public session API 没有静态证据表明能原样表达本项目的 Responses encrypted reasoning、签名字段、data URL 媒体落盘和 `file://media/...` 引用约束。
- 没有静态证据表明其 compaction 会保持当前工具结果剪裁、图片保留数、checkpoint 结构和中断工具补写语义。
- 没有静态证据表明其接受态、已接受稿和 `run.json` 能在无模型密钥下恢复并重新交付。
- 即便 provider/stream API 支持若干协议，也不能据此宣称当前 SSE、重试和三协议请求体可直接替换。

判断：**最适合做局部 loop/消息 API 候选，不适合直接替换本项目 session/runner。** 先保留当前 Transcript 和 runner；未来只能在 adapter 后进行离线 fixture 对照。

## 候选二：dimetron/pi-go

来源：

- [模块与 README（pkg.go.dev）](https://pkg.go.dev/github.com/dimetron/pi-go)
- [README 中的 internal/session 文档](https://pkg.go.dev/github.com/dimetron/pi-go/internal/session)
- [GitHub 仓库](https://github.com/dimetron/pi-go)
- [许可证页面](https://pkg.go.dev/github.com/dimetron/pi-go@v0.4.3?tab=licenses)

README 描述的是基于 Google ADK Go 的终端 coding agent，包含多 provider、沙箱工具、TUI、JSONL append-only event logs、branch、compaction 和 resume。pkg.go.dev 中的会话实现路径是：

`github.com/dimetron/pi-go/internal/session`

这是 `internal` package。根据 Go 的 internal import 规则，`compare-figure` 不在 `github.com/dimetron/pi-go` 的父树内，不能从外部模块 import 该包。即使 README 所述功能很接近“会话管理”，这也不是可嵌入的 public library API。

其公开文档还显示会话是 `baseDir/<session-id>/` 下的 `meta.json` 与 `events.jsonl`，使用 ADK session/event 类型；公开的 compaction API 包括 shed/summarize 和 token 估算。该格式与本项目 Transcript、runner v3 和 accepted snapshot 没有静态兼容证据。

判断：**排除作为嵌入库候选。** 除非上游未来把所需能力移出 `internal` 并稳定公开 API，否则不能以依赖方式复用其会话实现。

关于 Go 版本：本次 `web_fetch` 没有成功取得其 raw `go.mod`；README 页面中出现的版本要求应视为未独立确认的信息，不能把它写成本项目已验证兼容。

## 候选三：OrdalieTech/pigo

来源：

- [模块与 README（pkg.go.dev）](https://pkg.go.dev/github.com/OrdalieTech/pigo)
- [public codingagent API](https://pkg.go.dev/github.com/OrdalieTech/pigo/codingagent)
- [public codingagent/session API](https://pkg.go.dev/github.com/OrdalieTech/pigo/codingagent/session)
- [许可证页面](https://pkg.go.dev/github.com/OrdalieTech/pigo@v0.4.9?tab=licenses)
- [GitHub 仓库](https://github.com/OrdalieTech/pigo)

### 可 import 的 public package

README 明确把 pigo 定位为 SDK-first，并给出：

```go
import "github.com/OrdalieTech/pigo/codingagent"

result, err := codingagent.NewAgentSession(codingagent.AgentSessionOptions{})
```

pkg.go.dev 还列出：

- `github.com/OrdalieTech/pigo/ai`
- `github.com/OrdalieTech/pigo/agent`
- `github.com/OrdalieTech/pigo/agent/harness`
- `github.com/OrdalieTech/pigo/codingagent`
- `github.com/OrdalieTech/pigo/codingagent/session`
- provider、extension、tools 等 public 包

`codingagent/session` 公开 `SessionManager`、`SessionEntry`、`FileEntry`、`LoadEntriesFromFile`、`ParseSessionEntries`、`MarshalJSONL`、`MigrateSessionEntries`，并声明当前版本为 3；`codingagent` 公开 `NewAgentSession`、`AgentSessionRuntime` 和运行时的 prompt/continue/compact/abort 等 API。

### 与当前项目的匹配

匹配点：

- 三者中最明确地把 embedding SDK 当作一等目标。
- public session manager 能处理 JSONL tree、分支、compaction、迁移和内存/持久化会话。
- public runtime 能处理 prompt、continue、tool、abort 和 session lifecycle。
- MIT 许可证，适合做独立 adapter 评估。

风险和缺口：

- pigo 的“pi-compatible JSONL”指其 Pi v3 tree，不等于本项目既有 Transcript 格式或 `run.json`。
- `SessionManager`/`SessionEntry` 的树状消息与当前 `meta/msg/usage/checkpoint/state` 事件模型不同，不能直接打开已有会话而不丢状态。
- pigo runtime 自己拥有 provider、tool orchestration、compaction 和 session lifecycle；接入后容易把 drawing/comparison 的工具状态、accept 规则和无密钥恢复耦合进外部 runtime。
- 没有静态证据表明 pigo 会保留本项目的 reasoning encrypted content、签名回放、媒体文件引用、安全路径校验、未完成工具补写和 accepted snapshot 交付。
- `codingagent` 的高层 API 侵入面大于只替换 loop kernel；若只想减少会话维护，使用完整 runtime 可能得不偿失。

判断：**若目标是采用一套完整 Pi SDK，pigo 是最直接的嵌入入口；若目标只是减少当前通用 loop 维护，侵入性过大。** 在未完成双格式和行为对照前，不应迁移。

关于 Go 版本：本次 `web_fetch` 无法成功读取其 raw/proxy `go.mod`，所以不写未经确认的版本结论。

## 能否真的减少维护

只能减少一部分通用代码，不能减少生产工作流的主要维护面：

- 外部库可以潜在承担 provider registry、SSE 解码、通用消息类型、agent loop、分支会话和通用 compaction。
- 本项目仍须维护三协议的实际 provider 细节、thinking/signature/encrypted replay 的兼容、图片/媒体路径、工具批次、预算策略、checkpoint、runner fingerprint、LuaDraw 编译、尺寸测量、comparison 评分、submit/accept 规则和无密钥交付。
- 外部库升级还会增加依赖版本、格式迁移、行为回归和供应链审查成本。

因此当前判断是：**直接迁移不会让总维护量可靠下降；局部替换只有在明确减少重复代码并保留本项目契约时才可能值得。**

## 建议的迁移边界（仅供未来评审）

如果未来批准做实验，边界应是：

1. 在 `go/session` 外保留 `Transcript`、媒体落盘、checkpoint/state、accepted/no-key recovery。
2. 在 `go/runner` 外保留 run.json v3、输入指纹、恢复判定、交付和锁。
3. 将外部库限制在一个可替换 adapter：输入为模型消息/工具定义，输出为当前 `Completer` 能理解的 assistant message 和 usage/stream events。
4. 由项目自己的 adapter 将外部事件映射为当前 Transcript 事件；不让外部库直接写生产 JSONL。
5. 任何 compaction 只能先写实验格式并通过 golden fixture 对照，不能静默改变当前 checkpoint。
6. drawing/comparison、工具图像、尺寸、submit/accept 和接受态恢复不迁移、不重写。
7. 先用 faux/mock provider 和固定 SSE fixture 做离线测试；不得调用真实模型或读取凭据。
8. 只有在旧会话可读、处理中断可恢复、已接受结果无密钥可交付、三协议回放一致后，才讨论是否引入依赖。

## 最终选择

- **现在：不迁移、不安装、不改 `go.mod`。**
- **若只是回答“guanshan/pi-go 能不能嵌入”：能从 public package import，但它是 alpha，且不是当前格式的 drop-in session manager。**
- **若按嵌入 API 完整度排序：pigo > guanshan > dimetron（dimetron 的相关 session 是 internal，直接排除）。**
- **若按本项目低侵入局部替换排序：guanshan 的 public `packages/agent`/`packages/ai` 更适合先评估；pigo 高层 SDK 更完整但更容易侵入当前 runner/session 边界。**
- **不应重写 `drawingcomparison` 或接受态，也不应把外部 Pi v3 JSONL 当作当前会话格式。**

本评估仅代表来源阅读后的静态结论，不代表任何候选与 compare-figure 已兼容或测试通过。
