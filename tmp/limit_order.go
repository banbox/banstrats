package tmp

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
)

// LimitOrder 限价单策略：当没有订单时，按价格的90%创建限价单，3个bar未成交则取消
func LimitOrder(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum: 10,
		OnBar: func(s *strat.StratJob) {
			e := s.Env

			// 检查是否有活跃订单，如果有直接返回
			if len(s.LongOrders) > 0 || len(s.ShortOrders) > 0 {
				return
			}

			// 没有活跃订单时，创建限价单
			// 获取当前价格的90%作为限价买入价格
			currentPrice := e.Close.Get(0)
			limitPrice := currentPrice * 0.997

			// 创建限价买入订单，使用StopBars设置3个bar后自动取消
			s.OpenOrder(&strat.EnterReq{
				Tag:      "limit_buy",
				Limit:    limitPrice, // 限价单价格
				StopBars: 3,          // 3个bar未成交则取消
			})
		},
	}
}
