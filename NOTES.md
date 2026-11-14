# NOTES · sforum.qq-bot

设计取舍、核对结论与踩过的坑。面向「下一个接手的人」，不是用户文档（那是
`README.md`）也不是接手速查（那是 `AGENTS.md`）。

## 1. 为什么独立仓库，而不是放进 SForum 主仓库

宿主主仓库已经有 `extensions/builtin/` 与 `extensions/optional/`。QQ 机器人不放进去，
理由有三条：

1. **发布节奏不同**。主仓库跟宿主版本走；QQ 机器人有自己的版本与兼容矩阵，
   不必每次宿主发版都跟着动。
2. **信任边界不同**。带后端的包要走 exact-artifact 信任确认。主仓库内置插件是宿主
   自己审过的代码，而外部仓库的包需要宿主之外的审核环节。
3. **依赖方向不同**。插件依赖宿主 SDK，反过来会让宿主依赖某个具体插件。

代价是宿主要额外扫一个「外部扩展源根」。这条能力已经存在
（`EXTERNAL_EXTENSION_ROOTS`），不新增机制。

## 2. 参考仓库的选择

骨架结构抄了两家，不抄业务内容：

- `sforum-web-search` —— 同类插件仓库（仓库根即扩展源根），它的 `.gitignore`、
  `scripts/prepare-package.sh`、`ci.yml` / `release.yml` 的骨架最贴近本仓库形态，
  直接沿用其目录约定与命名。
- `sforum-app-store` —— 它的 `ci.yml` / `release.yml` 提供了「verify → build → publish」
  三段式与品牌资产门禁的语义参照；`LICENSE` 的版权主体与它保持一致（`Inkedus`）。

## 3. 骨架阶段的 CI 行为

`backend/` 与 `sforum.extension.json` 尚未落地。若 CI 无条件跑 `go vet ./...`，
第一次推送就会红。因此两个工作流都做了**存在性判断**：

- 后端 module 不存在 → 跳过 Go 构建与测试，只发一条 `::notice` 说明尚未就位；
- 清单不存在 → 跳过 `extension digest --write` / `validate` / `test`。

这是刻意的：骨架阶段的红灯没有信息量，而绿灯 + 一条说明能真实反映「结构已就位、
业务未就位」这个状态。后端落地后，这些判断自然走真分支，不需要再改工作流。

## 4. 产物命名：为什么是 `sforum.qq-bot`

- **扩展 id**（清单里的 `id`、包目录名、CI 的 `PACKAGE_DIR`）：`sforum.qq-bot`。
  它必须落在 `<id>.` 命名空间下，且清单里 `id` 与包目录名一致。
- **发布产物名**（zip 文件名）：`sforum.qq-bot_<version>_<os>_<arch>.zip`。
  与扩展 id 同名，而不是仓库名 `sforum-qq-bot`。理由：用户装的是「包」不是「仓库」，
  产物名应当指向包本身；仓库名里的连字符只是 GitHub 的仓库命名约束。

## 5. 摘要与 exact-artifact

宿主按清单里的 `backend.digest` 与包内可执行文件的实际摘要做比对，对不上就拒绝启用。
这带来两个必须记住的约束：

1. 构建参数三处必须完全一致（宿主 dev watcher、`scripts/prepare-package.sh`、CI）：
   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`。任何一处不同，同一份源码在两边
   产出不同字节，摘要永远不收敛。
2. **改了包内任何文件（含品牌图）都要重跑 `prepare-package.sh`**。摘要变了，旧的信任授权
   即失效，后台会显示「升级」，必须重新走一次「授权当前精确制品 → 确认并升级」。
   同版本号只改内容时，卡片上的「重启」不会切换制品——重启不是升级。

## 6. 已知要核对的宿主约束（实现阶段逐条验证）

以下是读宿主源码得到的结论，实现时需要逐条复验，不要照抄：

1. **插件设置以 `SFORUM_SETTING_<KEY>` 环境变量注入子进程**（大写、非字母数字转下划线），
   而且从未保存过的设置不会被注入——插件必须自带与清单 `default` 一致的默认值。
2. **插件子进程的环境是白名单**：`HTTP_PROXY` / `HTTPS_PROXY` 不会透传，
   出站代理只能来自插件自己的设置项。
3. **`aiTools[].contractVersion` 必须严格是 `<id>@1`**；`id` 与 `handler` 必须落在
   `<扩展 id>.` 命名空间下。
4. **Go 的 RE2 不支持 `\uXXXX` 转义**，必须写 `\x{...}`。写错的后果是
   `regexp.MustCompile` 在包初始化时 panic，插件连 go-plugin 握手都进不去。
5. **宿主 `DigestTree` 禁止符号链接**：不要用软链把包目录挂到别处。

## 7. 品牌资产

`brand/` 下的三张图由设计师产出，规格沿用宿主约定：

| 文件 | 尺寸 | 上限 |
| --- | --- | --- |
| `brand/icon.svg` | `viewBox="0 0 512 512"`，自包含 | 40 KiB |
| `brand/icon.png` | 512×512 PNG | 120 KiB |
| `brand/social-preview.png` | 1280×640 PNG | 600 KiB |

生成与校验走宿主脚本 `SForum/scripts/brand/generate.mjs`；门禁命令是
`go run ./cmd/sforum extension brand check <包路径>`，已串进 `prepare-package.sh`。

品牌文件不进 `packageFiles`，但属于包树：改图会改变包摘要，走正常升级流程。
**不要为了消掉摘要漂移去改版本号。**

## 8. 凭据边界

QQ 开放平台的 AppID / AppSecret 只走后台设置页（`secret` 类型），由宿主以
`SFORUM_SETTING_*` 注入子进程。以下位置一律不放凭据：

- 仓库文件（含 `NOTES.md`、示例配置）；
- 扩展清单 `sforum.extension.json`；
- 提交进 git 的 `.env`。

理由很直接：清单与安装包是公开可下载的，把凭据写进去等于把它发出去。

## 9. 待验证的问题（实现阶段）

1. QQ 侧长连接（WebSocket）与 HTTP 回调两种接入方式，哪种更适合长驻的论坛实例？
   倾向长连接：省掉公网回调地址的部署成本，但需要处理断线重连与多实例抢占。
2. 多个论坛实例同时指向同一个 QQ App 时，事件会被多份消费。是否需要在插件侧做
   租约，还是交由运营者保证一个 App 只对一个站点？
3. 入站回流是否应该默认关闭。默认开启意味着任何群成员发言都可能变成论坛内容，
   这在权限模型上是危险的。倾向默认关闭 + 显式关键词前缀 + 显式群白名单。
