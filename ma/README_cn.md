## [demo](demo.go)
一个简单的双均线交叉策略，金叉开多平空，死叉开空平多

## [ma_er](ma_er.go)
结合均线和ER效率指标的简单策略

## [stoploss](stoploss.go)
演示如何在开单时设置止损，每个bar更新止损，以及在更小周期更新止损

## [takeprofit](takeprofit.go)
演示如何在开单时设置止盈，每个bar更新止盈，以及在更小周期更新止盈

## [info_bars](info_bars.go)
订阅其他品种/周期数据

## [draw_down](draw_down.go)
一个跟踪回撤止盈的例子，在订单最大盈利超过10%后，盈利区间亏损50%退出。

## [custom_exit](custom_exit.go)
针对订单，使用更复杂的逻辑控制何时平仓出场。

## [edit_pairs](edit_pairs.go)
监听品种列表，任意修改或排序后返回，可用于动态控制需交易的品种列表

## [batch](batch.go)
综合多个品种，统一控制并计算全局信息，然后控制某些品种开仓平仓的例子

## [websocket](websocket.go)
订阅交易所Websocket高频数据的示例，支持：K线，订单簿深度，逐笔交易

## [post_api](post_api.go)
演示如何在实盘时，通过web api接受外部数据请求，用于策略辅助判断或直接开平仓，您可从TradingView中配置此api以便自动下单
