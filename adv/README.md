[中文](README_cn.md)

## [draw any chart with chart.js](genAnyChart.go)
start command:
```shell
bot chart demo -symbol ETH/USDT
```
Read the specified period data of the specified variety, and use chart.js to draw any type of HTML chart (taking a line chart as an example)

## [draw klineChart with indicators](genKlineChart.go)
start command:
```shell
bot chart kline -pairs ETH/USDT
```
Read the specified period data of the specified variety, use [klinechart](https://klinecharts.com/) to draw K-line charts, and add custom indicators to display
> For most cases, it is recommended that you start the webUI and then visit `http://localhost:8000/kline` to experience a more feature-rich K-line chart
