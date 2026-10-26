# 2026-10-27 上游安全更新同步记录

## 确认到的上游更新

- console：`b37f0c4` → `b003030ccb53209bf56977596dbc6c0d6cfb71e4`，显示版本 v2.6.2-Beta.14。
- core：`76008b92ec2c` → `dca6f5d65861c6a35b8cb857ab134f617cb6410c`，上游标注 v2.0.5.6；Go 模块固定为 `v0.0.0-20261026155051-dca6f5d65861`。
- console 的发布工作流明确说明“更新 go 版本避免高风险漏洞”。这段说明没有给出具体 CVE 编号，不能自行补造漏洞编号。
- 上游要求 Go 1.27.3。前序本地 v0.4.2 候选包已经使用 Go 1.27.3，但尚缺此次最新 core。
- 新旧 core 源码差异集中于海旗科技：题型与 recordId 解析、简答/填空接口、正式提交/交卷与只保存的区分及对应测试；并非仅修改版本字符串。

参考：

- https://github.com/yatori-dev/yatori-go-console/compare/b37f0c4...b003030
- https://github.com/yatori-dev/yatori-go-core/compare/76008b92ec2c...dca6f5d65861

## 本地完成的工作

1. 将本地 console 快进到上述提交。原 go.mod/go.sum 的本地修改先保存到 Git stash，并在本地 release/security-sync-20261027 中保留备份；没有丢弃用户改动。
2. 桌面模块固定新 core，更新 go.sum；Android 重新导入新构建的 v0.4.2 mobilecore。
3. Windows CI 按 go.mod 安装 Go，并固定 console 源码提交；Docker 测试镜像改为 Go 1.27.3 官方基础镜像，不再继承旧 0.3.4 自定义镜像。
4. EXE、AAR、APK 全部重新构建；从 EXE 和 Android libgojni.so 的 Go build info 核实 Go 1.27.3 及新 core，不是只改文件名。
5. 应用版本统一为 v0.4.2，Android versionCode 42；更新 README、CHANGELOG 和校验清单。
6. 保留账号默认开关、标准版全局上限 5、现有 Android UI 以及定时/日志功能。

## 验证及扫描边界

- `go test ./... -count=1`、`go vet ./...` 通过。
- 上游海旗 API / aggregation 两个包的测试通过。
- console 目录执行 `go mod verify`：all modules verified。桌面根目录的同命令遇到本地 replace 模块 `yatori-go-console v0.0.0` 的 missing ziphash 错误，不能将该检查写成通过；CI 使用带 go.sum 校验的 `go mod download`，并保留测试步骤。
- 新 core 的模块校验值与上游 go.sum 一致：`h1:iRX8HPub6VMLuVi50EcpzzqAc/U64VUvVcYDmGH8zGc=`。
- 工具：govulncheck v1.9.0；数据库 https://vuln.go.dev，工具报告数据库更新时间 2026-10-21。
- 扫描 Windows 主包、service、mobilecore 源码及 EXE / Android native core。源码扫描仅报告 `GO-2026-5932` 的模块级 OpenPGP 弃用提醒，未给出可达调用链；Windows 实际源码依赖列表不包含 openpgp 包。
- 两份二进制扫描仍输出同一 OpenPGP 公告的包级和通配符符号级条目。不能仅依据退出码 0 宣称无漏洞，也不能据此证明应用存在可利用调用链；保留原始 JSON 供后续分析。本轮未更改整个 x/crypto 依赖来掩盖提醒。
- Docker 引擎未启动，未实际构建或运行新测试镜像。
- 未将新 APK 安装到手机，也未重新实跑真实账号的所有任务。前序候选包的用户基本测试结果不能代替此次所有平台的端到端验证。

## 旧文件与发布状态

- 同名旧 v0.4.2 产物已经备份到本地 release/security-sync-20261027/previous-v0.4.2；标准输出路径替换为最新构建。
- 对旧 max999 EXE 的二进制扫描命中 35 个不同公告，涉及旧 Go 标准库及旧依赖（公告命中不等同于已证明可利用）；最新产物扫描只剩上述 OpenPGP 提醒。
- 自用 max999 EXE 原样保留。实际 build info 仍为 **Go 1.26.4 + core cf6134fdb2bd**，没有获得本次更新，应另行重建后再使用。
- 未删除旧 Go、未永久修改系统 PATH。上述验证完成时尚未推送、打 tag 或上传；后续发布状态以 GitHub v0.4.2 Release 为准。
- 产物元数据中的 `desktopCommit=6f3b9d0` 记录构建时基线 HEAD；构建包含当时尚未提交的 v0.4.2 工作区改动，不能把该字段当作完整发布源码 SHA。完整源码由本轮发布提交及 v0.4.2 tag 标识，不改写既有产物来伪称重新构建。
