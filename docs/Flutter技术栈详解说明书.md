<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

# RIMS Flutter 技术栈详解说明书

## 1. 文档定位

### 1.1 读者对象

本文面向已经熟悉 Unity、C#、游戏对象、Prefab、生命周期、资源打包和多平台发布，但尚未系统接触 Flutter、Dart、Android、iOS 应用开发的工程师。

本文不是 Flutter 官方教程的替代品，而是一份结合 RIMS 项目的技术栈说明书，目标是让 Unity 工程师能理解：

- Flutter 是什么，和 Unity 的思维差异在哪里。
- Dart 语言需要掌握哪些核心能力。
- 一个 Flutter 移动端项目由哪些层组成。
- Android/iOS 发布链路和 Unity 移动端发布有什么不同。
- RIMS 这种库存管理 App 应该怎样选型、分层、接后端和落地工程。

### 1.2 与现有文档的关系

- `docs/前端需求文档-v2.md` 定义 RIMS 前端产品范围、业务模块、接口集成和 Flutter 技术路线。
- 本文档解释 Flutter 技术栈本身，帮助具备 Unity 背景的开发者理解后续为什么这样设计。
- 后续如创建 `rims-flutter/` 工程，应以 v2 需求文档为产品约束，以本文档为技术栈入门和工程约定参考。

### 1.3 一句话结论

Flutter 可以理解为一个专门为业务 App 构建的跨平台 UI 引擎和应用框架。它不像 Unity 那样以实时场景、游戏循环、物理、渲染管线为中心，而是以 Widget 树、声明式 UI、状态变化、平台插件和原生发布链路为中心。

对于 RIMS 这类库存管理 App，Flutter 比 Unity/Godot 更贴合表单、列表、扫码、报表、文件、安全存储、系统权限、App Store/Play Store 发布和长期维护。

## 2. Flutter 和 Unity 的核心差异

### 2.1 思维模型对照

| Unity 概念 | Flutter 近似概念 | 关键差异 |
|---|---|---|
| Scene | Route/Page | Flutter 页面是 Widget 树，不是持续运行的场景 |
| GameObject | Widget 或业务对象 | Widget 通常不可变，描述 UI 长什么样 |
| Component/MonoBehaviour | Widget、State、Controller、Provider | Flutter 不依赖每帧 Update 驱动业务 |
| Prefab | 自定义 Widget | 复用 UI 时组合 Widget，而不是实例化场景对象 |
| Transform/Layout | Layout Widget、Constraints | Flutter 由父级约束子级尺寸，布局是约束传播 |
| Canvas/UI Toolkit | Widget + RenderObject + Material/Cupertino | Flutter UI 全部走 Widget 体系 |
| Update() | 状态变化触发 rebuild | 不是每帧写逻辑，而是事件驱动和响应式刷新 |
| Coroutine | Future、async/await、Stream | 异步任务更像 C# async，而不是帧协程 |
| ScriptableObject | 常量配置、ThemeData、DTO、枚举、JSON 配置 | Flutter 配置通常用 Dart 对象和代码生成 |
| AssetBundle/Addressables | assets、pubspec.yaml、远程资源 | 移动业务 App 的资源通常更轻量 |
| Native Plugin | Flutter Plugin、Platform Channel | 插件通过 Dart API 封装 Kotlin/Swift 能力 |
| Player Settings | Android/iOS 工程配置、pubspec.yaml、flavors | 每个平台有更多原生配置细节 |
| Build APK/IPA | flutter build apk/aab/ipa | iOS 仍必须依赖 macOS + Xcode 签名链路 |

### 2.2 最重要的认知切换

Unity 常见写法是“场景中有对象，对象有脚本，脚本在生命周期中改变对象状态”。Flutter 常见写法是“状态发生变化，框架根据状态重新计算 Widget 树，并高效更新屏幕”。

在 Flutter 里，不要把页面想成一个你手动维护的对象树。更好的理解是：

```text
业务状态 + 页面参数 + 主题配置 -> build() -> 当前应显示的 Widget 树
```

当状态变化时，Flutter 会重新调用相关区域的 `build()`。你负责描述“此时 UI 应该是什么样”，而不是直接命令某个按钮、文本或列表节点逐项改变。

### 2.3 Update 循环不是主角

RIMS 不是游戏，不需要每帧轮询库存、单据或网络状态。绝大多数业务行为来自：

- 用户点击、输入、扫码、下拉刷新。
- HTTP 请求成功或失败。
- 本地缓存读取完成。
- 权限、仓库、网络状态变化。
- 系统生命周期变化，例如 App 进入后台或恢复前台。

因此 Flutter 开发重点不是写 `Update()`，而是写清楚状态来源、状态流转、异常处理和 UI 对状态的响应。

## 3. Flutter 技术栈全景

### 3.1 Flutter SDK

Flutter SDK 包含：

- Dart SDK 和运行时。
- Flutter Framework，也就是常用的 Widget、动画、手势、布局、主题、导航等 API。
- Flutter Engine，负责渲染、文本、输入、平台消息和底层运行。
- 命令行工具，例如 `flutter doctor`、`flutter pub get`、`flutter run`、`flutter build`。
- Android/iOS/macOS/Windows/Linux/Web 等平台工程模板。

开发时，Flutter 使用 JIT 和 Hot Reload 提高迭代速度。发布时，移动端通常使用 AOT 编译，把 Dart 代码编译为原生机器码。

### 3.2 Dart 语言

Dart 是 Flutter 的主要开发语言。对 C# 工程师来说，Dart 不难上手，但要特别注意下面这些点：

- 默认空安全，变量能否为空要在类型上明确表达。
- `async/await`、`Future` 与 C# 的 `Task` 直觉接近。
- `Stream` 用于连续异步事件，例如扫码结果、网络状态、表单变化。
- 没有 Unity 那种 `MonoBehaviour` 生命周期，页面生命周期由 Widget 和路由管理。
- 包管理使用 `pubspec.yaml` 和 `pub.dev`。
- 常用代码生成工具包括 `build_runner`、`json_serializable`、`freezed`。

示例：

```dart
class Product {
  const Product({
    required this.id,
    required this.code,
    required this.name,
    this.barcode,
  });

  final int id;
  final String code;
  final String name;
  final String? barcode;
}

Future<Product> fetchProduct(int id) async {
  final response = await api.get('/products/$id');
  return Product.fromJson(response.data);
}
```

`String? barcode` 表示条码可能为空。`required` 表示创建对象时必须传入该参数。

### 3.3 Widget 系统

Flutter UI 的基本单位是 Widget。Widget 是对界面的声明，不等同于屏幕上的真实节点。框架会根据 Widget 创建 Element 和 RenderObject 来完成挂载、布局和绘制。

常见 Widget 类型：

| 类型 | 用途 |
|---|---|
| `StatelessWidget` | 只依赖外部参数，不保存内部可变状态 |
| `StatefulWidget` | 有内部状态或生命周期，例如输入框、Tab、相机扫码页 |
| `InheritedWidget`/Provider | 向子树传递状态 |
| Layout Widget | `Row`、`Column`、`Stack`、`GridView`、`ListView`、`CustomScrollView` |
| Material Widget | `Scaffold`、`AppBar`、`NavigationBar`、`TextField`、`Dialog` |

RIMS 的商品卡片可以被写成一个可复用 Widget：

```dart
class ProductListTile extends StatelessWidget {
  const ProductListTile({
    super.key,
    required this.name,
    required this.code,
    required this.stockText,
    required this.onTap,
  });

  final String name;
  final String code;
  final String stockText;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      title: Text(name),
      subtitle: Text(code),
      trailing: Text(stockText),
      onTap: onTap,
    );
  }
}
```

这类似一个 UI Prefab，但更偏函数式组合。它不应该直接知道后端接口、仓库切换、JWT 等全局细节。

### 3.4 Material 和 Cupertino

Flutter 同时提供 Material 和 Cupertino 两套组件。

RIMS 建议以 Material 3 为主：

- 库存管理系统更偏生产力工具，Material 组件成熟、清晰、可维护。
- Android/iOS 可以共享同一套业务 UI，不必为每个平台完全重写。
- iOS 上只在必要位置使用 Cupertino 风格，例如日期选择、系统弹窗、滑动返回手感。

设计重点不是“看起来像 Android”或“看起来像 iOS”，而是保证操作效率、信息层级、可读性和跨平台一致性。

### 3.5 Flutter Engine 与渲染

Flutter 不使用系统原生控件绘制大部分 UI，而是通过自己的渲染引擎绘制界面。这一点和 Unity 更接近：Flutter 控制渲染结果，而不是把每个按钮都交给 Android/iOS 原生控件。

但 Flutter 和 Unity 的目标不同：

- Unity 面向实时 3D/2D、物理、游戏循环、复杂场景。
- Flutter 面向业务 UI、文本、布局、动画、输入、手势和平台集成。

这也是为什么 Flutter 在表单、列表、导航、主题、无障碍、测试、发布生态上更适合 RIMS。

## 4. RIMS 推荐技术栈

### 4.1 推荐组合

| 领域 | 推荐技术 | 对 Unity 工程师的解释 |
|---|---|---|
| UI 框架 | Flutter stable | 类似“业务 App 专用引擎 + UI 框架” |
| 语言 | Dart | 类似 C# 的现代空安全语言，上手成本较低 |
| 设计系统 | Material 3 + ThemeData | 类似统一 UI Style/Theme 配置 |
| 路由 | go_router | 类似场景/页面导航表，但支持鉴权重定向 |
| 状态管理 | Riverpod | 类似全局服务和可监听状态容器，但更可测试 |
| 网络 | Dio | 类似 HTTP 客户端加拦截器，用来统一 JWT 和错误处理 |
| JSON 模型 | json_serializable/freezed | 类似自动生成 DTO 和不可变数据模型 |
| 安全存储 | flutter_secure_storage | 保存 token 等敏感数据，优先走系统安全能力 |
| 本地数据库 | Drift(SQLite) | 缓存商品、库存、草稿、弱网任务 |
| 偏好设置 | shared_preferences | 存主题、声音、筛选偏好等轻量数据 |
| 扫码 | mobile_scanner 或 ML Kit 类插件 | 调用相机识别条码/二维码 |
| 文件能力 | image_picker、file_picker、path_provider、share_plus | 拍照、选文件、下载、分享 |
| 图表 | fl_chart 或 graphic | 销售趋势、库存总览、排行等 |
| 崩溃监控 | Sentry、Firebase Crashlytics 或国内等价方案 | 收集线上崩溃、错误和性能数据 |
| 构建配置 | flavors + dart-define | 类似为开发、测试、生产准备不同构建目标 |

### 4.2 为什么这些技术适合 RIMS

RIMS 的核心不是炫酷动画，而是高频、稳定、可追踪的业务操作：

- 登录鉴权。
- 仓库切换。
- 商品和库存列表。
- 销售、退货、入库、调拨、盘点、转换单据。
- 扫码录入。
- 附件上传和私有文件下载。
- 报表、导出、权限字段显隐。
- 弱网提示、草稿恢复、重复提交防护。

上面的技术组合优先解决这些问题：

- 用 Riverpod 管理“当前用户、当前仓库、当前权限、当前网络状态”。
- 用 Dio 拦截器统一加 `Authorization` 和 `X-Warehouse-ID`。
- 用 Drift 缓存商品、库存摘要和草稿。
- 用 go_router 处理登录态、权限和页面跳转。
- 用代码生成减少 DTO 手写错误。
- 用 Flutter 插件调用相机、文件、安全存储和系统分享。

## 5. Flutter 工程结构建议

### 5.1 目录结构

建议后续 Flutter 工程放在仓库根目录下的 `rims-flutter/`：

```text
rims-flutter/
  lib/
    main.dart
    app/
      bootstrap.dart
      router.dart
      env/
      theme/
    core/
      api/
      auth/
      cache/
      errors/
      logging/
      platform/
      telemetry/
      widgets/
    shared/
      dto/
      enums/
      formatters/
      validators/
    features/
      auth/
      home/
      warehouse/
      product/
      inventory/
      document/
      report/
      file/
      audit/
      settings/
    l10n/
  test/
  integration_test/
  assets/
  android/
  ios/
```

### 5.2 分层说明

| 层 | 职责 |
|---|---|
| `app/` | App 启动、路由、主题、环境配置 |
| `core/` | 网络、安全存储、缓存、日志、错误、平台能力等基础设施 |
| `shared/` | 跨模块 DTO、枚举、格式化器、表单校验器 |
| `features/` | 按业务模块组织页面、状态、仓储、组件 |
| `assets/` | 图片、图标、字体、启动页资源 |
| `test/` | 单元测试、Widget 测试 |
| `integration_test/` | 真机或模拟器上的端到端测试 |

### 5.3 单个业务模块内部结构

以库存模块为例：

```text
features/inventory/
  data/
    inventory_api.dart
    inventory_repository.dart
    inventory_cache_dao.dart
  domain/
    inventory_models.dart
    inventory_filters.dart
    inventory_status.dart
  presentation/
    inventory_list_page.dart
    inventory_detail_page.dart
    inventory_alert_page.dart
    widgets/
  application/
    inventory_controller.dart
    inventory_providers.dart
```

建议理解为：

- `data/` 负责“数据从哪里来”，比如后端 API 或本地数据库。
- `domain/` 负责“业务对象是什么”，比如库存状态、筛选条件。
- `presentation/` 负责“界面长什么样”。
- `application/` 负责“页面行为怎么编排”，比如加载、刷新、提交、错误处理。

这种拆分能避免把所有逻辑塞进一个页面文件，也便于测试。

## 6. 状态管理

### 6.1 为什么需要状态管理

Unity 中常见做法是用单例、Service、Manager、事件系统或 ScriptableObject 管理全局状态。Flutter 也需要管理状态，但要适应声明式 UI。

RIMS 至少有这些状态：

- 当前登录会话。
- 当前用户和角色。
- 当前仓库。
- 权限菜单和字段可见性。
- 商品、库存、单据、报表列表查询状态。
- 表单草稿。
- 网络状态和弱网重试状态。
- 上传和下载任务状态。

如果只靠 `setState()`，项目很快会变成页面之间互相传状态、刷新时机混乱、测试困难。因此推荐使用 Riverpod 或同等级方案。

### 6.2 Riverpod 的角色

Riverpod 可以理解为一个可测试、可组合、可监听的状态和依赖容器。

它可以表达：

```text
authProvider -> 当前登录状态
warehouseProvider -> 当前仓库
apiClientProvider -> 带 token 和仓库 header 的 HTTP Client
inventoryRepositoryProvider -> 库存仓储
inventoryListProvider -> 当前筛选条件下的库存列表
```

当用户切换仓库时，依赖仓库的库存、单据、报表 Provider 可以自动失效或刷新。

### 6.3 常见状态类型

| 状态类型 | 例子 | 推荐处理 |
|---|---|---|
| 临时 UI 状态 | Tab、展开折叠、输入框焦点 | Widget 内部状态 |
| 页面查询状态 | 商品列表筛选、分页、加载中、错误 | Feature Controller/Provider |
| 全局状态 | 登录、角色、仓库、主题 | 全局 Provider |
| 持久状态 | token、草稿、缓存 | Secure Storage/SQLite/KV |
| 连续事件 | 扫码流、网络状态变化 | Stream Provider 或插件回调封装 |

### 6.4 避免的做法

- 不要把所有状态放进一个巨大 `AppState`。
- 不要让 UI Widget 直接拼接 HTTP Header。
- 不要在每个页面里重复写 token 过期处理。
- 不要把库存变更请求在离线时静默排队自动重放。
- 不要把普通用户不可见的成本价字段缓存到明文本地数据库。

## 7. 网络与后端对接

### 7.1 RIMS 后端约束

RIMS 后端所有业务 API 位于 `/api/v1` 下，移动端需要遵守：

- 登录接口返回 JWT。
- 受保护接口携带 `Authorization: Bearer <token>`。
- 库存、非标库存、单据、流水、报表接口携带 `X-Warehouse-ID`。
- 响应使用统一信封：`code`、`message`、`data`、`traceId`。
- 业务成功以 `code == 0` 为准。
- 普通用户不能看到成本价、利润、库存总值等敏感字段。

### 7.2 API Client 职责

Flutter 侧应封装统一 API Client，职责包括：

- 设置 baseUrl。
- 自动附加 token。
- 自动附加当前仓库 ID。
- 解析统一响应信封。
- 将业务错误码转换为前端异常。
- 记录 `traceId`。
- 处理 401 会话失效。
- 处理超时、断网、证书、服务端异常。
- 上传下载文件时提供进度回调。

建议所有业务模块只依赖 Repository，不直接依赖 Dio。

```text
Page/Controller -> Repository -> ApiClient -> Dio -> RIMS Backend
```

### 7.3 错误码处理

| 错误码 | 前端行为 |
|---|---|
| `10001` | 清理会话，跳转登录，保留必要草稿 |
| `10002` | 提示无权限，刷新权限或返回上级页面 |
| `10003` | 显示表单校验错误，定位字段 |
| `10004` | 提示资源不存在，刷新列表 |
| `10005` | 提示重复数据，保留表单 |
| `20001` | 显示库存不足、商品、仓库、可用数量 |
| `20002` | 提示状态已变化，要求刷新 |
| `20003` | 提示重复提交或查询提交结果 |
| `50000` | 提示系统异常，展示 traceId |

### 7.4 文件接口注意点

RIMS 有公开文件和私有文件：

- 商品图片等公开资源可用 `/uploads/*` URL 展示。
- 私有文件必须通过 `/api/v1/files/:id/download` 下载，需要鉴权 Header。

Flutter 中如果直接把私有下载链接交给系统浏览器，可能无法携带 JWT。因此应通过 App 内 HTTP 下载，再调用系统分享或打开文件。

## 8. 本地缓存、草稿和弱网

### 8.1 哪些数据可以缓存

适合缓存：

- 商品基础信息。
- 仓库列表。
- 库存摘要。
- 最近查看记录。
- 报表摘要。
- 页面筛选偏好。
- 单据草稿。
- 低风险缩略图。

谨慎缓存：

- 成本价、利润、库存总值。
- 审计日志详情。
- 私有文件。
- 用户身份和权限快照。

禁止明文缓存：

- JWT。
- 后续如后端提供的刷新令牌。
- 密码。
- 普通用户不可见的敏感业务字段。

### 8.2 草稿策略

销售、退货、入库、调拨、盘点、非标转标准等流程可以保存草稿。草稿应包含：

- 用户 ID。
- 仓库 ID。
- 业务类型。
- 表单字段。
- 明细行。
- 创建时间和更新时间。
- 关联的本地草稿 ID。

用户切换账号或仓库时，不应直接混用草稿。

### 8.3 弱网策略

RIMS 的库存变更是强一致业务，不能像聊天消息那样随意离线排队自动重放。推荐策略：

- 查询类页面可以展示上次缓存，并标注更新时间。
- 表单可以保存草稿，等待网络恢复后由用户确认提交。
- 用户已经点击提交但结果未知时，进入“待确认”状态，不重复提交。
- 恢复网络后优先查询单据或流水确认结果。
- 对销售、调拨、盘点等库存变更，必须避免静默重放导致重复扣减或重复入库。

## 9. Android 和 iOS 开发环境

### 9.1 Windows 环境能做什么

在 Windows 上可以完成：

- Flutter 代码开发。
- Android 模拟器和真机调试。
- Android APK/AAB 构建。
- 与 WSL 中运行的 Go 后端联调。
- 大部分单元测试和 Widget 测试。

需要注意：本仓库 Go 后端命令按项目要求在 WSL 中运行，但 Flutter 移动端开发通常运行在宿主 Windows 上，因为 Android Studio、模拟器、USB 调试和桌面 IDE 更适合直接接入 Windows 环境。

### 9.2 iOS 需要 macOS

iOS 调试、签名、真机安装、TestFlight 和 App Store 发布需要：

- macOS。
- Xcode。
- Apple Developer Program 账号。
- Bundle Identifier。
- 证书和 Provisioning Profile。
- App Store Connect。

Windows 不能独立完成正式 iOS 构建和上架。可以在 Windows 开发 Flutter 业务代码，但最终 iOS 构建需要 Mac、本地 Mac mini、云构建服务或 CI 中的 macOS runner。

### 9.3 Android 发布链路

Android 发布通常需要：

- Android Studio。
- Android SDK、Build Tools、Platform Tools。
- 应用包名，例如 `com.example.rims`，正式项目前应使用公司域名反写。
- 签名 keystore。
- `flutter build appbundle` 生成 AAB。
- Google Play Console 开发者账号。

国内安卓渠道通常还需要额外适配各应用市场的包体、隐私合规、权限说明和加固要求。

### 9.4 iOS 发布链路

iOS 发布通常需要：

- Xcode 工程配置。
- Bundle ID。
- Signing & Capabilities。
- App Icons、Launch Screen。
- 隐私权限说明。
- Archive。
- 上传到 App Store Connect。
- TestFlight 测试。
- App Review。

iOS 审核更重视隐私、权限用途、账号登录、演示账号、数据采集说明和支付规则。RIMS 如果是企业内部使用，也可以考虑企业签名、MDM 或私有分发，但这会带来额外合规和账号成本。

## 10. Flutter 开发流程

### 10.1 常用命令

```bash
flutter doctor
flutter create rims_flutter
flutter pub get
flutter run
flutter test
flutter build apk
flutter build appbundle
flutter build ipa
```

说明：

- `flutter doctor` 用来检查环境。
- `flutter pub get` 拉取依赖。
- `flutter run` 运行到模拟器或真机。
- `flutter test` 运行测试。
- `flutter build apk` 生成 Android APK。
- `flutter build appbundle` 生成 Google Play 推荐的 AAB。
- `flutter build ipa` 生成 iOS 发布构建，需要 macOS/Xcode。

### 10.2 Hot Reload 和 Hot Restart

Hot Reload 类似 Unity 改脚本后快速刷新，但更适合 UI 和业务代码迭代：

- Hot Reload 保留当前 App 状态，注入代码变化。
- Hot Restart 重启 Dart VM，状态会清空。
- 原生平台配置、插件、权限、启动图、Gradle/Xcode 改动通常需要完整重启或重新构建。

### 10.3 Debug、Profile、Release

| 模式 | 用途 | 特点 |
|---|---|---|
| Debug | 日常开发 | 支持 Hot Reload，性能不代表线上 |
| Profile | 性能分析 | 接近 Release，可采集性能数据 |
| Release | 正式发布 | AOT 编译，关闭调试能力 |

不要用 Debug 模式判断最终性能，也不要只在模拟器上判断扫码、文件和相机体验。

## 11. 平台能力和插件

### 11.1 插件是什么

Flutter 插件通常由两部分组成：

- Dart API，供业务代码调用。
- Android/iOS 原生实现，通常是 Kotlin/Java 和 Swift/Objective-C。

当 Flutter 生态已有成熟插件时，优先使用插件。当插件不满足需求时，再通过 Platform Channel 自己封装原生能力。

### 11.2 RIMS 常用平台能力

| 能力 | 使用场景 | 注意点 |
|---|---|---|
| Camera | 扫码、拍商品图、拍单据附件 | 权限、对焦、弱光、不同机型兼容 |
| Gallery | 从相册选商品图 | iOS/Android 权限差异 |
| File Picker | 选择导入模板或附件 | MIME、扩展名、大小限制 |
| Secure Storage | 存 token | 不要降级到明文存储 |
| SQLite | 本地缓存和草稿 | 需要迁移策略 |
| Share/Open File | 导出报表后分享或打开 | 私有文件先鉴权下载 |
| Network Status | 弱网提示和恢复 | 不等于服务端可达，要结合探活 |
| Push Notification | 未来库存预警或审批提醒 | iOS 权限申请更敏感 |

### 11.3 插件选型检查清单

引入插件前应检查：

- 是否支持 Android 和 iOS。
- 是否支持当前 Flutter stable。
- 最近是否维护活跃。
- License 是否允许商业闭源 App 使用。
- 是否存在已知安全或隐私风险。
- 是否需要额外原生配置。
- 是否能通过中国大陆常见设备测试。
- 是否有可替代方案。

## 12. UI、布局和适配

### 12.1 Flutter 布局的关键概念

Flutter 布局遵循“约束向下传递，尺寸向上传递，父级决定位置”的模型。

常见误区：

- `Row` 中长文本不加 `Expanded`，导致溢出。
- 滚动容器中嵌套无限高度列表。
- 忽略安全区，导致内容被刘海、状态栏、底部手势条遮挡。
- 只按一种屏幕尺寸设计，没有考虑平板和横屏。

### 12.2 RIMS 的响应式策略

手机端：

- 底部导航或一级页面导航。
- 列表优先，详情进入新页面。
- 高频动作放在底部或右下角。
- 表单分组，避免单屏过载。

平板端：

- 可采用主从布局。
- 左侧列表，右侧详情。
- 报表页面可以展示更多筛选项和图表。
- 管理员页面可提高信息密度。

### 12.3 主题和设计 Token

RIMS 应把颜色、字号、间距、圆角、状态标签、危险操作样式做成统一主题配置，而不是散落在各页面中。

建议至少定义：

- 主色、成功色、警告色、错误色、中性色。
- 文本层级。
- 间距刻度。
- 表单控件尺寸。
- 状态标签样式。
- 列表密度。
- 图表配色。
- 浅色和深色主题。

## 13. 安全、权限和隐私

### 13.1 登录与 token

RIMS 当前后端使用 JWT。Flutter 侧应：

- 登录成功后把 token 存入安全存储。
- 每次请求由 API Client 自动附加 token。
- token 过期时统一清理会话并跳转登录。
- 不在日志中输出 token。
- 不把 token 存入普通 SQLite 或 shared_preferences。

### 13.2 权限显隐

前端需要做权限显隐，但这只是用户体验优化，不是最终安全边界。

必须以后端为准：

- 普通用户不展示用户管理、角色管理、仓库管理、非标库存管理、审计日志等入口。
- 普通用户不展示成本价、利润、库存总值等字段。
- 即使前端隐藏入口，后端仍必须拒绝越权请求。
- 前端收到 `10002` 时应刷新权限或返回上级页面。

### 13.3 隐私合规

移动 App 上架时，需要说明：

- 为什么申请相机权限。
- 为什么访问相册或文件。
- 是否收集崩溃日志、设备信息、网络信息。
- 数据是否上传服务器。
- 是否有第三方 SDK。
- 用户账号如何注销或停用。

这些不是 Flutter 本身的技术问题，但会直接影响上架审核。

## 14. 测试策略

### 14.1 测试类型

| 测试类型 | 测什么 | RIMS 示例 |
|---|---|---|
| Unit Test | 纯业务逻辑 | 错误码映射、金额格式化、库存状态判断 |
| Widget Test | 单个页面或组件 | 库存卡片、登录表单、权限字段显隐 |
| Integration Test | 真机流程 | 登录、切仓库、扫码、创建销售单 |
| Golden Test | UI 快照 | 关键组件视觉回归 |
| Manual QA | 真实设备验证 | 相机、文件、弱网、横竖屏、安装升级 |

### 14.2 必测流程

一期至少覆盖：

- 登录成功、登录失败、token 过期。
- 仓库切换后库存和报表刷新。
- 普通用户成本字段不可见。
- 销售单库存不足错误。
- 单据重复提交防护。
- 扫码查商品。
- 商品图片上传和展示。
- 私有附件下载失败和成功。
- 弱网下保存草稿。
- Android 真机和 iOS 真机核心流程。

### 14.3 Unity 工程师容易忽略的测试点

- 键盘弹出后表单是否被遮挡。
- iOS 底部手势条是否遮挡按钮。
- Android 返回键是否符合预期。
- App 从后台恢复后 token、仓库和页面数据是否仍正确。
- 长列表滚动是否掉帧。
- 低端机扫码是否稳定。
- 权限拒绝后是否有可恢复路径。

## 15. 构建、配置和多环境

### 15.1 环境划分

建议至少区分：

| 环境 | 用途 |
|---|---|
| dev | 本地开发，连接本地或开发后端 |
| test | 测试环境，供 QA 和联调用 |
| staging | 预发布，接近生产配置 |
| prod | 正式环境 |

### 15.2 配置方式

Flutter 常见配置方式：

- `--dart-define` 注入 API 地址、环境名、功能开关。
- flavors 区分 Android/iOS 的包名、应用名、图标和签名。
- CI/CD 根据分支或标签选择构建环境。

示例：

```bash
flutter run --dart-define=APP_ENV=dev --dart-define=API_BASE_URL=http://127.0.0.1:8080/api/v1
flutter build appbundle --flavor prod --dart-define=APP_ENV=prod
```

### 15.3 与后端联调

如果后端运行在 WSL 中，移动端访问要注意地址：

- Windows 浏览器访问可用 `127.0.0.1:8080`。
- Android 模拟器访问宿主机常用 `10.0.2.2:8080`。
- 真机访问需要电脑和手机在同一网络，使用电脑局域网 IP。
- iOS 模拟器在 macOS 上通常可访问 Mac 的 `localhost`，真机仍需局域网 IP 或测试环境域名。

生产环境必须使用 HTTPS，不应在正式包中保留明文 HTTP。

## 16. 学习路径建议

### 16.1 第一阶段：理解 Dart 和 Flutter UI

目标：能写简单页面和组件。

学习内容：

- Dart 基础语法、空安全、集合、类、枚举。
- `async/await`、`Future`、`Stream`。
- `StatelessWidget`、`StatefulWidget`。
- `Row`、`Column`、`ListView`、`Stack`、`Scaffold`。
- 表单、输入框、按钮、弹窗、导航。

练习：

- 写一个商品列表页面。
- 写一个商品详情页面。
- 写一个登录表单。
- 写一个库存状态标签组件。

### 16.2 第二阶段：接入真实后端

目标：能调用 RIMS Go 后端。

学习内容：

- Dio 基本用法。
- 拦截器。
- JSON 解析。
- 统一响应信封。
- 错误码映射。
- JWT 保存和附加。
- `X-Warehouse-ID` 仓库上下文。

练习：

- 登录获取 token。
- 请求当前用户信息。
- 请求仓库列表。
- 请求商品列表。
- 请求库存列表。

### 16.3 第三阶段：工程化

目标：能写可维护的业务模块。

学习内容：

- Riverpod。
- go_router。
- Repository 分层。
- DTO 代码生成。
- Drift/SQLite。
- 表单校验。
- 单元测试和 Widget 测试。

练习：

- 完整实现商品模块。
- 完整实现库存列表和库存预警。
- 实现销售单草稿。
- 写错误码映射测试。

### 16.4 第四阶段：平台能力和发布

目标：能做真机能力和发布包。

学习内容：

- Android 权限。
- iOS 权限。
- 相机扫码。
- 图片选择。
- 文件上传下载。
- Android 签名和 AAB。
- iOS 证书、Provisioning、TestFlight。
- 崩溃监控和日志。

练习：

- 真机扫码查商品。
- 拍照上传商品图片。
- 下载报表并分享。
- 构建 Android 测试包。
- 在 macOS 上跑通 iOS 真机调试。

## 17. Unity 工程师常见坑

### 17.1 把 Widget 当 GameObject 改

不要试图拿到某个 Widget 后直接改它的属性。Flutter 推荐通过状态变化驱动 UI 重建。

### 17.2 在 build() 里做副作用

`build()` 可能被频繁调用。不要在 `build()` 里发 HTTP 请求、写数据库、弹窗、导航或上传文件。副作用应放在 Controller、Provider、生命周期方法或明确的用户事件中。

### 17.3 忘记处理异步生命周期

页面发起异步请求后，用户可能已经返回上一页。需要避免在页面销毁后继续更新 UI。

### 17.4 低估平台权限

相机、相册、文件、通知、定位等权限在 Android 和 iOS 上差异很大。权限文案、拒绝后的引导和审核说明都要提前设计。

### 17.5 只在模拟器验证

扫码、拍照、文件、弱网、分享、安装升级、推送等能力必须真机验证。模拟器只能覆盖一部分。

### 17.6 把离线重试做得太激进

库存变更不是普通消息。销售、调拨、盘点等操作如果重复提交，可能造成真实库存错误。弱网下应保存草稿和待确认状态，而不是静默反复重放。

## 18. RIMS 一期 Flutter 落地范围建议

### 18.1 一期优先级

优先实现：

- 登录与会话。
- 仓库列表和切换。
- 首页快捷入口。
- 商品查询和条码查询。
- 库存列表、库存详情、低库存预警。
- 销售单、退货单。
- 商品图片上传。
- 基础报表查看。
- 普通用户与管理员权限显隐。

第二阶段实现：

- 入库、调拨、盘点。
- 非标库存和转标准。
- 审计日志。
- 报表导出和分享。
- 更完整的弱网草稿和待确认机制。
- 推送提醒和远程配置。

### 18.2 推荐第一个开发闭环

建议第一个端到端闭环为：

```text
登录 -> 获取当前用户 -> 获取可见仓库 -> 选择仓库 -> 商品列表 -> 库存列表 -> 扫码查商品
```

这个闭环能验证：

- Flutter 环境。
- 路由和登录态。
- JWT。
- 仓库上下文。
- Dio API Client。
- 列表 UI。
- RIMS 后端联调。
- 真机扫码插件。

完成这个闭环后，再扩展销售单、退货单和文件上传会更稳。

## 19. 成本和账号

### 19.1 Flutter 本身

Flutter SDK 和 Dart SDK 是开源免费使用的。Flutter 项目可以用于商业 App。实际成本主要来自设备、开发者账号、CI/CD、监控服务、云服务和第三方插件或 SDK。

### 19.2 Android 相关成本

常见成本：

- Android Studio 和 Android SDK 免费。
- Google Play Console 开发者账号官方帮助页显示需要 US$25 一次性注册费用，实际支付要求以注册地区和官方页面为准。
- 国内安卓市场可能有企业认证、软著、隐私合规、加固或渠道服务成本。
- 需要 Android 真机做扫码、相机、文件和弱网测试。

### 19.3 iOS 相关成本

常见成本：

- Xcode 免费，但需要 macOS 设备。
- Apple Developer Program 官方页面显示为 $99 年费；Apple Developer Enterprise Program 页面显示为 $299/year，后者主要用于组织内部私有分发场景。
- 真机调试、TestFlight、App Store 发布需要开发者账号。
- 如使用云构建或 Mac mini，会有额外成本。

## 20. 推荐阅读顺序

建议按以下顺序学习：

1. 先读 Flutter 官方“Get started”和“Widget”相关内容，跑通第一个 App。
2. 学 Dart 空安全、`Future`、`async/await`、集合和类。
3. 学 Flutter 布局、表单、导航和主题。
4. 学 Dio、JSON 解析和错误处理。
5. 学 Riverpod 和 go_router。
6. 学 Android/iOS 权限、签名和发布。
7. 回到 RIMS v2 文档，实现第一个端到端闭环。

## 21. 官方参考资料

- Flutter 官网：[https://flutter.dev/](https://flutter.dev/)
- Flutter 官方文档：[https://docs.flutter.dev/](https://docs.flutter.dev/)
- Flutter 架构指南：[https://docs.flutter.dev/app-architecture](https://docs.flutter.dev/app-architecture)
- Flutter 响应式与自适应：[https://docs.flutter.dev/ui/adaptive-responsive](https://docs.flutter.dev/ui/adaptive-responsive)
- Flutter Platform Channels：[https://docs.flutter.dev/platform-integration/platform-channels](https://docs.flutter.dev/platform-integration/platform-channels)
- Flutter 测试文档：[https://docs.flutter.dev/testing](https://docs.flutter.dev/testing)
- Flutter Android 发布：[https://docs.flutter.dev/deployment/android](https://docs.flutter.dev/deployment/android)
- Flutter iOS 发布：[https://docs.flutter.dev/deployment/ios](https://docs.flutter.dev/deployment/ios)
- Dart 官网：[https://dart.dev/](https://dart.dev/)
- Dart 语言文档：[https://dart.dev/language](https://dart.dev/language)
- pub.dev 包仓库：[https://pub.dev/](https://pub.dev/)
- Apple Developer Program：[https://developer.apple.com/programs/](https://developer.apple.com/programs/)
- Google Play Console 帮助：[https://support.google.com/googleplay/android-developer](https://support.google.com/googleplay/android-developer)

## 22. 简短决策建议

如果你是 Unity 工程师，学习 Flutter 的最佳方式不是先完整啃完移动端原生开发，而是按 RIMS 的业务闭环边做边学：

```text
Dart 基础 -> Flutter Widget -> 路由和状态 -> HTTP 接口 -> 本地缓存 -> 扫码文件 -> Android/iOS 发布
```

Unity 经验会帮助你理解跨平台、资源、构建、性能和插件边界。但在 Flutter 中，最需要重新建立的是声明式 UI、状态驱动界面、移动端系统权限和应用商店发布链路。
