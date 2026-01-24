package tmp

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

// TriggerEnt 触发入场：当没有订单时，按价格的90%创建限价单，3个bar未成交则取消
func TriggerEnt(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum: 0,
		OnBar: func(s *strat.StratJob) {
			e := s.Env

			// 检查是否有活跃订单，如果有直接返回
			if len(s.LongOrders) > 0 || len(s.ShortOrders) > 0 || s.IsWarmUp {
				return
			}

			// 没有活跃订单时，创建限价单
			// 获取当前价格的90%作为限价买入价格
			currentPrice := e.Close.Get(0)
			limitPrice := currentPrice * 0.9997

			log.Info("trigger at", zap.Float64("price", limitPrice))
			// 创建限价买入订单，使用StopBars设置3个bar后自动取消
			s.OpenOrder(&strat.EnterReq{
				Tag:      "test",
				Short:    true,
				Stop:     limitPrice,
				StopBars: 5, // 5个bar未成交则取消
			})
		},
		OnOrderChange: func(s *strat.StratJob, od *ormo.InOutOrder, chgType int) {
			log.Info("order change", zap.Int64("order_id", od.ID), zap.Int("chg_type", chgType))
		},
	}
}
