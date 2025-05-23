[English](./README.md)

# Banbot 策略
这个代码仓库包含了适用于 [banbot](https://github.com/banbox/banbot) 的免费策略。

| 文件夹                           | 说明                        |
|-------------------------------|---------------------------|
| [ma](ma/README_cn.md)         | 示例策略&各种例子                 |
| [grid](grid/README_cn.md)     | 网格策略                      |
| [adv](adv/README_cn.md)       | banbot高级用法示例              |
| [rpc_ai](rpc_ai/README_cn.md) | 通过grpc与python交互，支持ML/DL模型 |

## 免责声明
这些策略仅用于教育目的。请勿投入你无法承担损失的资金。使用本软件风险自负。作者及所有关联方对你的交易结果不承担任何责任。

务必先通过回测来测试策略，然后以模拟交易（Dry-run）的方式运行交易机器人。在你了解其工作原理以及预期的盈利/亏损情况之前，请勿投入真实资金。

我们强烈建议你阅读 [文档](https://docs.banbot.site/zh-CN/guide/start) 并了解这个机器人的运行机制。

有些策略仅在特定的市场条件下有效，而其他一些则是更具 “通用性” 的策略。值得注意的是，根据所使用的交易平台和交易对，进一步的优化可能会带来更好的效果。

请记住，回测结果在很大程度上取决于所使用的交易品种、时间框架和时间范围——所以请运行你自己的回测，使其与你的实际使用场景相符，以便自行评估每个策略。

## 分享你自己的策略
我们欢迎大家贡献经典的算法交易策略，以便更多人能够快速上手使用 banbot。

## 常见问题
### 什么是 banbot？
banbot 是一个用 Go 语言编写的免费开源加密货币交易机器人。它旨在提供一个简单、易用、高性能的量化回测体验。
它包含了 WebUI 界面、实盘DashboardUI 界面、回测功能、绘图工具、资金管理工具以及策略优化功能。

### 如何测试一个策略？
你可以直接将这个代码仓库用作你的项目，或者将策略的 Go 源码文件复制到你的策略项目中并注册。

然后可以执行 `go build -o bot` 命令，将 banbot 和策略编译成一个单一的可执行文件，

* 然后配置yml，执行`bot backtest`即可执行回测
* 也可直接运行这个文件，就可以从 `http://localhost:8000` 访问 WebUI了
* 或者查看[命令文档](https://docs.banbot.site/zh-CN/guide/bot_usage)探索更多用法

