[中文](./README_cn.md)

# Banbot Strategies
This repo contains free strategies for [banbot](https://github.com/banbox/banbot).

| Directory                  | Description                                               |
|----------------------------|-----------------------------------------------------------|
| [ma](ma/README.md)         | Example strategies & various examples                     |
| [idea](idea/README.md)     | Classic Strategy                                          |
| [grid](grid/README.md)     | Grid trading strategies                                   |
| [adv](adv/README.md)       | Examples of advanced usage of banbot                      |
| [rpc_ai](rpc_ai/README.md) | Interaction with Python via gRPC, supporting ML/DL models |

## Disclaimer
These strategies are for educational purposes only. Do not risk money which you are afraid to lose. USE THE SOFTWARE AT YOUR OWN RISK. THE AUTHORS AND ALL AFFILIATES ASSUME NO RESPONSIBILITY FOR YOUR TRADING RESULTS.

Always start by testing strategies with a backtesting then run the trading bot in Dry-run. Do not engage money before you understand how it works and what profit/loss you should expect.

We strongly recommend you to read the [document](https://docs.banbot.site/en-US/guide/start) and understand the mechanism of this bot.

Some only work in specific market conditions, while others are more "general purpose" strategies. It's noteworthy that depending on the exchange and Pairs used, further optimization can bring better results.

Please keep in mind, results will heavily depend on the symbols, timeframe and timerange used to backtest - so please run your own backtests that mirror your usecase, to evaluate each strategy for yourself.

## Share your own strategies
We welcome contributions of classic algorithmic trading strategies so that more people can quickly get started with banbot.

## FAQ
### What is banbot?
banbot is a free and open source crypto trading bot written in golang. It aims to provide a simple, easy-to-use, high-performance quantitative backtesting experience. 
It contains web ui, backtesting, plotting and money management tools as well as strategy optimization.

### How to test a strategy?
You can directly use this code repository as your project, or copy the Go source code files of the strategy into your strategy project and register them.

Then, you can execute the `go build -o bot` command to compile the banbot and the strategy into a single executable file.

* Then configure the yml file, and execute `bot backtest` to perform the backtest.
* You can also directly run this file, and then access the WebUI from `http://localhost:8000`.
* Or check the [command documentation](https://docs.banbot.site/zh-CN/guide/bot_usage) to explore more usage scenarios.

