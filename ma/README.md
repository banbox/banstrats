[中文](README_cn.md)

## [demo](demo.go)
A simple dual moving average crossover strategy. Open long and close short positions when a golden cross occurs, and open short and close long positions when a death cross occurs.

## [ma_er](ma_er.go)
A simple strategy that combines moving averages and the ER (Efficiency Ratio) indicator.

## [stoploss](stoploss.go)
Demonstrates how to set a stop - loss when opening a position, update the stop - loss for each bar, and update the stop - loss on a smaller time frame.

## [takeprofit](takeprofit.go)
Demonstrates how to set a take - profit when opening a position, update the take - profit for each bar, and update the take - profit on a smaller time frame.

## [info_bars](info_bars.go)
Subscribe to data of other varieties/periods.

## [draw_down](draw_down.go)
An example of tracking drawdown for take - profit. Exit the position when the maximum profit of an order exceeds 10% and then the profit drops by 50% within the profit range.

## [custom_exit](custom_exit.go)
For orders, use more complex logic to control when to close the position.

## [edit_pairs](edit_pairs.go)
Listen to the list of varieties, modify or sort it arbitrarily and then return it. It can be used to dynamically control the list of varieties for trading.

## [batch](batch.go)
An example of integrating multiple varieties, uniformly controlling and calculating global information, and then controlling the opening and closing of positions for certain varieties.

## [post_api](post_api.go)
It demonstrates how to accept external data requests through a web API during live trading. This can be used for auxiliary judgment of trading strategies or directly for opening and closing positions. You can configure this API in TradingView to enable automatic order placement. 
