
## [使用chart.js绘制任何图表](genAnyChart.go)
启动命令:
```shell
bot chart demo -symbol ETH/USDT
```
读取给定品种的指定周期数据，使用chart.js绘制任何类型的HTML图表（以折线图为例）

## [绘制带指标的K线图](genKlineChart.go)
启动命令:
```shell
bot chart kline -pairs ETH/USDT
```
读取给定品种的指定周期数据，使用[klinechart](https://klinecharts.com/)绘制K线图，并添加自定义指标显示
> 对于大多数情况，建议您启动webUI，然后访问`http://localhost:8000/kline`体验更丰富的K线图
