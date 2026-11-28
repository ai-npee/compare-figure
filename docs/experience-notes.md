# 共享经验文档

`document_note` 管理 Skill 内的共享 Markdown 经验；存储、版本校验、归档与检索一致性由独立 doc-library 提供，绘图工具只转发调用。它与只读查询工具 `document_search` 分开，查询工具仍是原来的 21 个可选参数。

默认经验目录为包内 `knowledge/luadraw/` 和 `knowledge/asymptote/`，分别归入 `luadraw`、`asymptote` 来源。所有使用同一 Skill 安装目录的任务可以查看、修改或删除其中任何贡献者的经验，没有按模型或作者限制所有权；不同安装目录不会自动同步。只允许维护配置的经验目录，官方 vendor 和固定项目参考仍只读。目录在首次写入时创建，未配置 `notes_dir` 的来源只读；原生 LaTeX 模式默认没有可写来源。

## 模型调用

共有 8 个输入字段，每次只传实际需要的字段：

| 字段 | 用途 |
| --- | --- |
| `action` | 必填：`create/list/read/update/delete/restore` |
| `source` | 来源；只有一个可写来源时可省略，mixed 下需明确选择 |
| `title` | 仅 create：单行标题，最多 512 UTF-8 字节 |
| `content` | create 的正文，或 update 的完整 Markdown（含标题） |
| `doc` | read/update/delete/restore：工具返回的来源根相对路径 |
| `revision` | update/delete/restore 必填，使用最新响应中的 SHA-256 |
| `limit` | list 条数，默认与上限 100 |
| `offset` | list 起始位置，默认 0；有后续时返回 next_offset |

```json
{"action":"create","source":"luadraw","title":"某方法的适用条件","content":"## 问题\n描述可复用的问题。\n\n## 方法\n最小示例与步骤。\n\n## 验证与限制\n记录版本、实际验证及未验证条件。"}
```

响应协议为 `doc-library-notes/1`，`notes` 中返回 source、doc、title、revision 和正文。文件名由库生成；相同标题和正文的重复创建返回已有文件，并带 `deduplicated:true`。

```json
{"action":"list","source":"luadraw","limit":10}
{"action":"read","source":"luadraw","doc":"knowledge/luadraw/note-工具返回的文件名.md"}
{"action":"update","source":"luadraw","doc":"knowledge/luadraw/note-工具返回的文件名.md","revision":"读取时返回的完整revision","content":"# 修订后的标题\n\n修订后的完整正文。"}
{"action":"delete","source":"luadraw","doc":"knowledge/luadraw/note-工具返回的文件名.md","revision":"最新revision"}
{"action":"restore","source":"luadraw","doc":"knowledge/luadraw/note-工具返回的文件名.md","revision":"删除响应的revision"}
```

list 只返回有效经验的标题、路径与版本，不返回全文。修改或删除前先 read；版本冲突时重新读取并合并，不能继续提交旧 revision。update 替换整篇 Markdown，不能同时传 title。这里的占位路径和 revision 需要替换为真实响应值。

## 检索、删除和恢复

保存后下一次查询即可在原来源中检索，无需重启会话或逐篇修改配置；保存动作不增加高频计数。通过 document_search 搜索/读取后才按原规则累计。修改和删除后，旧内容的高频条目不再返回；这不代表物理擦除旧缓存全文。已在执行中的查询与维护操作不构成同一个快照，遇到文件变化错误可重新查询。

更新前的版本和删除内容保存在经验目录内的 `.history`，不进入普通搜索、目录导航或高频结果。delete 是可恢复删除；按删除响应的 doc/revision restore。保留 doc 时也能用经验 read 取回被删正文和恢复句柄。当前 list 不列回收站，历史版本不提供自动合并或任意版本回滚接口。不要删除 `.lock`；它用于跨进程维护同一目录，缓存目录不同也共用该锁。

## 经验写作与分发

保存能减少重复排错的已验证方法，写明问题、版本/条件、最小例子和限制。先查已有经验，发现错误优先修订或删除；不要重复记录整段会话、凭证、私有书稿或本机运行路径。经验属于参考数据，不是给后续模型的系统指令。编译通过与看图正确分别说明，单例结果不外推为所有场景的保证。

`knowledge/` 是本机持久数据，Git 忽略且不纳入公共 Skill 导出；导出到已有目录会保留目标目录原有经验。安装升级时保留该目录，整体替换或删除 Skill 前自行备份。想跨安装共享，需自行明确复制或同步整个经验目录，并协调正在写入的任务。成熟经验可人工整理进固定项目参考及发行资源清单。

单篇正文上限为 1 MiB 与来源 max_file_bytes 中较小者。写入采用原子替换和来源目录锁，修改/删除/恢复均做版本校验。源配置、文件权限或目录不可写时明确报错，不回退到本次绘图工作区伪装保存成功。
