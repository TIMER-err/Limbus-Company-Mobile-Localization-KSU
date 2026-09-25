# Limbus Company 移动端汉化 · KernelSU 模块

面向已 Root 的 Android 设备，免 Termux、MoveCertificate 与 Clash。模块在本机生成专属 CA，通过本地 HTTPS 代理替换汉化资源，并可通过模块管理器在线更新汉化资源。

资源来自 [`TIMER-err/Limbus-Company-Mobile-Localization`](https://github.com/TIMER-err/Limbus-Company-Mobile-Localization)。

## 要求

- arm64 Android 设备
- KernelSU / SukiSU
- 已安装并启用一个元模块，例如官方 `meta-overlayfs` 或 Mountify

安装器会验证 KernelSU 当前活动的元模块；未安装、已禁用、正等待卸载或元数据无效时会立即中断安装。

## 安装

1. 从本仓库 Release 下载 `limbus-localization-ksu.zip`。
2. 在 KernelSU / SukiSU 管理器中安装。
3. 安装器显示确认提示后，按音量加键允许生成并挂载设备专属 CA；音量减键取消。
4. 安装完成后重启。

首次安装会下载并校验最新的 `localize_jp.zip` 与 `manifest.json`。以后在模块管理器中执行本模块的操作脚本即可更新；资源切换为原子操作，游戏读取期间不会看到半成品。

模块自身通过 `update.json` 接收 KernelSU / SukiSU 管理器的更新提示；汉化资源更新与模块程序更新互相独立。

模块描述会在开机和资源更新后刷新，显示代理、当前汉化版本、CA 挂载与域名映射状态。

## 系统安全

- 不写入 `/system`、`/vendor` 或 APEX 原始分区。
- Android 13 及以下的系统 CA 由元模块挂载。Mountify 会刻意跳过含 `hosts` 的模块，因此 `hosts` 在元模块完成后复制到 `/dev` 临时文件、只追加一个本地映射，再 bind mount 到原路径；不会写入原文件。
- Android 14 及以上会在开机早期复制完整 Conscrypt CA 目录到临时目录，加入模块 CA 后 bind mount 回原路径。原目录内容不会被修改。
- CA 私钥只保存在 `/data/adb/limbus-localization/ssl`，权限为 root-only，不会上传。
- 汉化资源先验证 Release 中的 SHA-256、ZIP CRC 和 manifest JSON，再切换为当前版本。
- 卸载模块会停止代理并清理模块数据；重启后所有临时挂载消失。

## 工作方式

模块将 `downloadcommon.limbuscompanycdn.org` systemless 映射到 `127.0.0.1`。本地代理只替换 `LocalizePatchInfo.json` 和 `localize_jp.zip`，其他路径通过经过证书校验的 HTTPS 回源到官方 CDN。

## 本地构建

需要 Go 1.24+、Bash 和 `zip`：

```bash
./scripts/build.sh
```

产物位于 `dist/limbus-localization-ksu.zip`。
