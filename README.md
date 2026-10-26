# Yatori Go Desktop

Yatori Go Desktop 是基于 [yatori-dev/yatori-go-console](https://github.com/yatori-dev/yatori-go-console) 改造的 Windows 桌面版工具。

原项目提供命令行和网页模式的学习任务执行能力。本项目在其基础上增加 Wails 桌面 GUI、账号管理、任务控制、日志中心、课程进度、全局设置、主题切换、公告、版本检测和题库配置等能力，让主要操作可以通过窗口完成。

> 本项目由 [yatori-dev/yatori-go-console](https://github.com/yatori-dev/yatori-go-console) 改造而来。感谢原项目作者和相关开源依赖。
> <img width="1782" height="1161" alt="image" src="https://github.com/user-attachments/assets/f33e531f-581c-49d1-91d4-9164d1be8b28" />
> <img width="1782" height="1161" alt="image" src="https://github.com/user-attachments/assets/648fbdff-c5da-456f-961f-81510bd47806" />



## 当前版本

v0.4.2

## v0.4.2 更新重点

- 同步上游 `yatori-go-console` 至 `b003030`，核心依赖锁定到 `yatori-go-core` 的 `dca6f5d65861`。
- 新增账号默认开启章节测试，默认关闭作业、考试；已明确设置的旧账号开关保持不变。
- 前端、配置归一化和任务执行层使用一致的默认开关，避免界面与实际执行不一致。
- 标准桌面版和 Android 核心的全局并发账号上限保持为 **5**。
- 桌面、前端、移动核心和 Android 统一版本为 v0.4.2；记录核心来源与产物 SHA-256。
- 延续 v0.4.1 的独立任务定时、日志刷新和提交重试能力，本次不重新设计 Android UI。
- 详细变更及验证边界见 [更新日志](CHANGELOG.md) 和 [v0.4.2 发布说明](docs/v0.4.2-release.md)。

## 本项目做了什么

- 使用 Wails v2 + React + TypeScript 构建 Windows 桌面界面。
- 复用 `yatori-go-console` 的核心 Go 逻辑，并通过 worker 子进程隔离平台任务，避免平台异常直接关闭主窗口。
- 提供账号管理、任务控制、日志中心、课程进度、全局设置、关于本项目等页面。
- 支持学习通、英华学堂、海旗科技、WeLearn 随行课堂、智慧职教等平台的桌面任务入口。
- 支持课程进度展示，包含课程、任务点、完成数量、进度条和部分平台的容错刷新。
- 支持学习通 AI / 外置题库答题配置，并提供通用第三方题库适配和 OCS 配置粘贴导入。
- 支持多套主题，默认主题改为灰白黑半透明磨砂风格，并保存到 `config.yaml`。
- 支持 GitHub 自动检测新版本、远程公告、本地公告已读记录。
- 配置、数据库和日志默认保存到 `%APPDATA%\yatori-go-console`。

## 功能状态

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 桌面 GUI | 支持 | Wails v2 桌面窗口 |
| 账号管理 | 支持 | 增删改账号和平台配置 |
| 任务控制 | 支持 | 启动、停止、状态展示、worker 并发限制 |
| 日志中心 | 支持 | 历史日志、实时日志、等级过滤 |
| 课程进度 | 支持 | 学习通、英华、海旗、WeLearn、智慧职教等已接入 |
| 全局设置 | 支持 | AI、题库、邮件、日志、主题、公告等配置 |
| 主题切换 | 支持 | 多套主题，写入 config.yaml |
| 版本检测 | 支持 | 从 GitHub Releases/Tags 检测新版本 |
| 第三方题库 | 支持 | 预设题库 + 自定义接口 + OCS 配置导入 |
| 学习通 | 已实测可用 | 支持任务执行、日志、停止、章节测验答题等流程 |
| 英华学堂 | 已实测可用 | 可正常登录并进入课程流程 |
| 海旗科技 | 已实测可用 | worker 子进程运行正常 |
| WeLearn 随行课堂 | 已实测可用 | 支持学时模式和完成度模式 |
| 智慧职教（ICVE） | 已接入并持续完善 | 支持 Cookie 登录、课程进度、测验/作业识别、题库/AI 答题和保存/提交诊断 |
| 其他原项目平台 | 支持入口 | 复用原项目 logic，具体效果需按账号继续实测 |

## 项目架构

```text
yatori-go-desktop/
├── main.go                         Wails 入口，支持普通桌面模式与 --worker 子进程模式
├── app.go                          Wails 绑定层，暴露给前端调用的 Go 方法
├── worker.go                       任务 worker 入口，按平台分发执行逻辑
├── worker_utf8_windows.go          Windows worker 输出编码处理
├── mobilecore/                     Android gomobile 核心导出层、JSON API 和 schema
├── service/
│   ├── account_service.go          账号 CRUD、仪表盘数据
│   ├── config_service.go           config.yaml 读写、默认值、校验
│   ├── log_service.go              日志缓冲、文件 tail、乱码修复
│   ├── task_manager.go             任务生命周期、worker 子进程启动与停止
│   ├── course_service.go           课程进度平台分发入口
│   ├── course_xxt.go               学习通课程进度适配
│   ├── course_yinghua.go           英华学堂课程进度适配
│   ├── course_hqkj.go              海旗科技课程进度适配
│   ├── course_welearn.go           WeLearn 随行课堂课程进度适配
│   ├── question_bank.go            通用第三方题库适配
│   ├── platform.go                 平台支持状态表
│   ├── xxt_runner.go               学习通安全 runner
│   ├── yinghua_runner.go           英华学堂 runner
│   └── other_runners.go            其他平台 worker 入口
├── frontend/
│   ├── src/components/             布局、弹窗、动画下拉、标签输入等组件
│   ├── src/pages/                  仪表盘、账号、任务、课程、日志、设置、关于页面
│   ├── src/lib/api.ts              前端调用 Wails Go 方法的封装
│   ├── src/lib/theme.ts            主题列表与主题应用
│   ├── src/lib/update.ts           GitHub 版本检测
│   └── src/lib/version.ts          当前版本和仓库地址常量
├── docs/mobilecore-release.md      Android mobilecore 发布说明
├── scripts/build-mobilecore.ps1    Android AAR、schema 和校验文件构建脚本
├── Dockerfile                      测试用 Docker 镜像定义
└── wails.json                      Wails 项目配置和 exe 元信息
```

## 数据路径

```text
%APPDATA%\yatori-go-console\
├── config.yaml
├── yatori.db
└── assets\log\
```

## 从源码构建

前置要求：Go 1.27.3+（与 go.mod 保持一致）、Node.js 18+、Windows 可用 GCC、Wails v2。

```bat
cd yatori-go-desktop
go vet ./...
go test ./service/... -v
cd frontend && npm install && npm run build && cd ..
wails build -platform windows/amd64 -o yatori-go-desktop.exe
```

## Android 核心产物

Android 版不重复维护平台业务逻辑，只消费桌面仓库发布的 `mobilecore` 产物。

```powershell
.\scripts\build-mobilecore.ps1 -Version 0.4.2 -Target android/arm64 -ApiSchemaVersion 1
```

产物默认输出到 `release/`：

```text
yatori-mobile-v0.4.2.aar
api-schema.json
yatori-core-version.json
yatori-mobilecore-checksums.json
```

详细说明见 [docs/mobilecore-release.md](./docs/mobilecore-release.md)。

## 已知限制

- 各平台实际任务执行能力取决于原项目 `logic/{platform}` 的实现和平台接口状态。
- AI 答题、外置题库依赖用户自行配置可用 API Key 或题库接口。
- 不提供验证码破解、人脸识别绕过、考试作弊等能力。
- 本项目仅用于个人已授权账号的学习任务管理和技术交流。

## 免责声明

本项目代码已开源，仅供学习与技术交流使用，严禁任何形式的倒卖、贩卖或商业牟利。

请在遵守法律法规、平台规则和账号授权范围的前提下使用本软件。任何个人或组织使用本项目代码或软件进行的违法违规行为，均与项目作者无关，相关责任由使用者自行承担。

如本项目内容对相关公司或平台造成影响，请通过 GitHub 仓库联系，我会及时处理。

## 更新日志

见 [CHANGELOG.md](./CHANGELOG.md) 或 [Releases](https://github.com/yuanycr/yatori-go-desktop/releases)。

## 致谢

- [yatori-dev/yatori-go-console](https://github.com/yatori-dev/yatori-go-console) - 核心学习逻辑来源
- [Wails](https://wails.io/) - Go + Web 桌面框架
- [React](https://react.dev/) + [TypeScript](https://www.typescriptlang.org/)

## License

本项目遵循 MIT License。原项目版权归原作者所有。

