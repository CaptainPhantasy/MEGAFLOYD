# 开始使用

## Floyd Code CLI 是什么

Floyd Code CLI 是一个运行在终端中的 AI Agent，帮助你完成软件开发任务和日常的终端操作——阅读和修改代码、执行 Shell 命令、搜索文件、抓取网页，并在执行过程中根据反馈自主规划和调整下一步行动。

它适用于以下场景：

- **编写和修改代码**：实现新功能、修复 bug、完成重构
- **理解项目**：探索陌生的代码库，解答架构和实现层面的问题
- **自动化任务**：批量处理文件、运行构建与测试、串联多个脚本

整套 CLI 以 TypeScript 编写，运行在 Node.js 之上。

## 安装

Floyd Code CLI 从本仓库的检出目录运行：目前还没有发布预编译二进制，也没有发布 npm 包。需要 Node.js 24.15.0 或更高版本，以及 pnpm 10.33.0。

::: tip 安装之前
Floyd Code CLI 为全交互式 TUI 应用，推荐在支持真彩色与连字的现代终端中运行以获得最佳体验，例如 [Kitty](https://sw.kovidgoyal.net/kitty/) 或 [Ghostty](https://ghostty.org/)。
:::

先确认两个版本：

```sh
node --version
pnpm --version
```

克隆仓库并安装 workspace：

```sh
git clone https://github.com/CaptainPhantasy/f7.git
cd f7
pnpm install
```

> Windows 用户首次启动前还需要安装 [Git for Windows](https://gitforwindows.org/)，Floyd Code CLI 会使用其中的 Git Bash 作为 Shell 环境。如果 Git Bash 安装在非标准路径，请把 `FLOYD_SHELL_PATH` 设为 `bash.exe` 的绝对路径。

可执行文件是 `f7`。在检出目录里，`pnpm dev:cli` 启动的是同一个 CLI，因此本页示例中出现的 `f7` 都可以换成它。

## 第一次启动

进入项目目录后直接运行 `f7` 启动交互界面：

```sh
cd your-project
f7
```

只想执行一条指令而不进入交互界面时，使用 `-p`：

```sh
f7 -p "帮我看一下这个项目的目录结构"
```

继续上一次会话加 `-c`：

```sh
f7 -c
```

Floyd Code CLI 不需要账号，用你想用的任意模型供应商即可。在交互界面里输入 `/provider` 可以跟着引导配置：选一个已知的第三方供应商、粘贴 API 密钥，再选默认模型。也可以自己写进 `~/.floyd-code/config.toml`：

```toml
default_model = "my-gateway/gpt-4.1"

[providers.my-gateway]
type = "openai"
base_url = "https://your-gateway.example/v1"
api_key = "YOUR_API_KEY"

[models."my-gateway/gpt-4.1"]
provider = "my-gateway"
model = "gpt-4.1"
max_context_size = 1047576
```

任何 OpenAI 兼容端点都可接入，Anthropic API 和 Google Gemini API 同样支持。[平台与模型](../configuration/providers.md)介绍了每种类型，以及如何从 catalog 或 registry 一次性导入供应商，而不必手写这些字段。

::: tip 凭证存放在哪里
密钥从 `config.toml` 读取，而不是从 shell 环境变量读取；唯一的例外是 `api_key_env`，它指向一个由你指定的变量名。详见[环境变量](../configuration/env-vars.md)、[配置文件](../configuration/config-files.md)和[配置覆盖](../configuration/overrides.md)。
:::

## 第一个对话

配置好供应商后，用自然语言描述任务即可。先让它熟悉当前项目：

```
帮我看一下这个项目的目录结构，简单介绍一下每个目录是做什么的
```

Floyd Code CLI 会自动调用文件读取、搜索等工具浏览相关内容后给出回答。只读操作默认自动执行无需确认；对于会修改文件或执行 Shell 命令的操作，默认会在执行前征求确认。

也可以直接描述更具体的任务：

```
在 src/utils 里新增一个函数，用来把任意字符串转成 kebab-case，并补一个单元测试
```

Floyd Code CLI 会规划步骤、修改代码、运行测试，并在每一步告诉你它做了什么。

::: tip 不知道能做什么？输入 `/help`
随时在输入框输入 `/help`，可以打开内置的命令和快捷键面板，按 `↑`/`↓` 翻看，`Esc` 关闭。退出时输入 `/exit`，或按 `Ctrl-C` 两次，或在输入框为空时按 `Ctrl-D`。
:::

## 常用命令与快捷键速查

第一次使用时，记住下面这些就够了：

**会话相关命令**

| 命令 | 说明 |
| --- | --- |
| `/new` | 开启新会话，清空当前上下文 |
| `/sessions` | 浏览历史会话，选择恢复 |
| `/model` | 切换当前使用的模型 |
| `/compact` | 手动压缩上下文，释放 token |
| `/fork` | 派生当前会话为保留完整历史的独立副本（仍停留在当前会话） |

**最常用快捷键**

| 快捷键 | 说明 |
| --- | --- |
| `Esc` | 中断流式输出 / 关闭弹窗 |
| `Ctrl-C` | 中断输出；空闲时连按两次退出 |
| `Shift-Tab` | 切换 Plan 模式 |
| `Ctrl-S` | 输出中途插入消息，无需等待结束 |
| `Ctrl-O` | 折叠 / 展开工具输出和压缩摘要 |

想看完整列表，输入 `/help` 或访问[斜杠命令参考](../reference/slash-commands.md)和[键盘快捷键](../reference/keyboard.md)。

## 数据存放在哪里

Floyd Code CLI 的本地数据默认保存在 `~/.floyd-code/` 下，包含配置文件、会话记录、日志和更新缓存。如需迁移到别处，通过 `FLOYD_CODE_HOME` 环境变量指定新路径。完整说明见[数据路径](../configuration/data-locations.md)和[环境变量](../configuration/env-vars.md)。

## 升级与卸载

安装完成后，验证 CLI 能否启动：

```sh
f7 --version
```

**升级**：在检出目录里 `git pull` 后重新运行 `pnpm install` 即可。`f7 upgrade` 面向打包安装：它会检查最新版本并展示更新选项。

**卸载**：删除检出目录即可。

## 下一步

- [交互与输入](./interaction.md) — 输入框操作、审批流程、Plan 模式和 "Ask When Needed" 模式详解
- [会话与上下文](./sessions.md) — 恢复会话、上下文压缩、导出会话
- [常见使用案例](./use-cases.md) — 典型任务的 prompt 示例
