# 更新日志

## v1.0.3

- 增加 KernelSU / SukiSU 模块在线更新地址。
- 明确显示汉化资源更新结果，并保留结果窗口 8 秒。
- 保留 v1.0.2 的 Mountify hosts 兼容和 PID 复用修复。

## v1.0.2

- 兼容 Mountify 对包含 `system/etc/hosts` 模块的主动排除规则。
- 校验 PID 对应的可执行文件，避免开机时因 PID 复用漏启代理或误结束其他进程。

## v1.0.0

- 首个独立 KernelSU / SukiSU 模块版本。
