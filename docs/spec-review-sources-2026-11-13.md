# 规格审查：官方来源与最小证明任务

核查日期：2026-11-13。对象：`specs/2026-11-13-open-scada-paid-ems.md`。仅查官方文档；未调用收费 API、未运行产品、未做真机试验。下文“建议”是工程推论，不冒充官方保证。

## 1. DeepSeek：已支持图像，不等于会可靠修改组态

**确认。** [Vision 文档](https://api-docs.deepseek.com/guides/vision) 对 `deepseek-flash` 使用原文 “accepts images alongside text”；支持 “JPEG, PNG, GIF, and WebP”。旧标识 `deepseek-v4-flash-vision-exp` 对应模型已退役，请求转到最新 Flash，不能把可用别名当成固定模型版本。[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion) 的用户消息允许文本/图片内容数组，图片使用 `image_url`；Vision 明确该接口不接受 system/assistant 图片。

截图相关限制：base64/远程 URL 单图 32 MiB，文件 ID 单图 64 MiB，请求体 48 MiB；`low` 缩为 512×512。指南另外描述推理前自动缩放、保持宽高比，大图约缩至 1300×1300 的面积量级；不能将 `original` 理解成端到端像素不变。本次确认的是官方托管 API，不外推到所有型号或消费端应用。

**未知。** 标注定位、小字识别、多轮局部改动及点位绑定正确率没有质量保证；本次也未验证账号可用性、延迟或真实效果。

**规格影响／最小证明。** 保留第4节及 T05 的“能力≠质量”，不要新增“不支持图片”的错误阻塞。实施前记录端点、模型标识、日期与实际版本信息；预算另获批准后，用固定截图集覆盖小字、箭头、重叠和连续两轮修改，检查目标修改、未修改区域回归与绑定正确性。模型漂移和工程产物可重复交付是两件事，不必建设通用模型平台。

## 2. 任意 HTML/JS：隔离不是净化

**确认。** [MDN iframe](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/iframe) 说明不授予 `allow-same-origin` 时使用特殊来源；同源父子页面同时授予 `allow-scripts` 与 `allow-same-origin`，生成代码可移除 sandbox，原文警告 “making it no more secure than not using the sandbox attribute at all”。还建议不可信内容使用独立来源，防止离开 iframe 后失去约束。

[MDN CSP](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP) 明说 “Setting a CSP is not an alternative to sanitizing input.” 但本项目本来就要运行生成脚本：删除危险 HTML 不能代替执行边界，给脚本加哈希也不证明脚本安全。sandbox 不等于禁网，更不保证 CPU、内存或 GPU 配额；CSP 的 `default-src` 也不是所有行为的统一开关。

**规格影响／最小证明。** 将隔离要求覆盖预览和已发布运行版。最小方案从独立来源、`sandbox="allow-scripts"` 和宿主不可被生成内容放宽的响应头 CSP 出发；逐项明确脚本、资源、连接、表单、嵌套页面及导航权限，而非仅写“已清洗”。T06 加恶意页面测试：父 DOM/存储访问、鉴权 API 请求、外连、弹窗/导航和资源耗尽；要求控制保护仍由服务端执行。不得凭 iframe 承诺卡死页面也绝不影响宿主。

## 3. 平台桥接：消息通道不是授权

**确认。** [MDN postMessage](https://developer.mozilla.org/en-US/docs/Web/API/Window/postMessage) 要求具体 `targetOrigin`，接收端核对 `origin`、必要时 `source`，并 “always verify the syntax of the received message”。它不是权限系统。opaque-origin 场景不能照抄普通来源校验，亦不能把共同的 `null` 来源视作可信身份。

**建议／最小证明。** 凭据留在可信宿主/后端；每次操作校验项目、对象、动作、权限和参数，生成页面只获最小数据与操作能力。桥接设计明确 iframe 身份、导航/重载后的失效规则；不能仅靠消息自报用户。测试伪造消息、兄弟 iframe、旧页面、越权对象及预览请求，断言真实设备无写入。这是单一受控接口的边界要求，不需新增通用插件权限框架。

## 4. Three.js：离线与显卡兼容必须分别验收

**确认。** [WebGLRenderer 官方文档](https://threejs.org/docs/pages/WebGLRenderer.html) 原文：“This renderer uses WebGL 2 to display scenes.”、“WebGL 1 is not supported since `r163`.” `capabilities` 描述当前上下文的纹理尺寸、采样等上限；`forceContextLoss/Restore` 依赖 `WEBGL_lose_context` 扩展。文档没有全 GPU、工控机或固定帧率保证。

**未确认。** 本次安装手册直链返回 404，[手册入口](https://threejs.org/manual/#en/installation) 仅返回索引，故没有获得可引用的 npm/离线打包正文；不将记忆中的安装步骤包装成核查结果。亦未检查现有工程的完整资源闭包。

**规格影响／最小证明。** 保留 T08，不承诺“支持浏览器即可”。在固定浏览器/驱动/目标机器上，冷缓存、断外网安装同一产物，检查 Three.js、addons、纹理、字体、模型及用到的解码器/worker 均本地可用；记录网络请求。另测 WebGL2 不可用和上下文丢失，必须显示明确失败且不误导控制状态；性能数字来自目标机测试，不凭库名新增 GPU 保证。

## 5. RTU：PTY 可验证协议路径，不能模拟电气证据

**确认。** [Modbus Serial Line V1.02](https://www.modbus.org/file/secure/modbusoverserial.pdf)，§2.5.1.1（第13页）要求帧间至少 3.5 字符时间，帧内超过 1.5 字符间隔应丢弃；≤19200 baud 严格按字符时间，>19200 推荐 750µs / 1.750ms。§2.5.1.2（第14页）规定 CRC16，低字节先发，并检查地址归属。

§3.3.2（第22页）：“at any time one driver only has the right for transmitting”，两线 RS485 还需 Common；§3.4.5–6（第27–28页）要求总线两端终端匹配，按设备需要确定偏置。这些都不是字节数组语义。

**规格影响／最小证明。** 保留虚拟 RTU、物理串口、目标设备三层证据；不换成 TCP。T03 用正式串口路径注入错 CRC/站号、分片、间隔、断线和迟到响应，记录实际字节与时间。PTY 不证明波特率真的生效、UART 校验位、半双工方向切换、线缆终端/偏置或抗干扰；OS 调度下的软件时间戳也不是线上的测量。物理层另用目标适配器、RS485 总线及测量工具确认帧间隔、收发切换和断线恢复，再做目标设备反馈验收。
