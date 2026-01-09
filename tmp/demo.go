package tmp

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	ta "github.com/banbox/banta"
	"go.uber.org/zap"
)

func Demo(pol *config.RunPolicyConfig) *strat.TradeStrat {
	// 定义可调超参数
	smlLen := pol.DefInt("smlLen", 5, core.PNorm(3, 10))
	maRate := pol.Def("maRate", 4, core.PNorm(2, 8))

	// 计算长期均线长度
	bigLen := int(float64(smlLen) * maRate)

	return &strat.TradeStrat{
		WarmupNum:    100, // 预热期100根K线
		EachMaxLong:  1,   // 最多开1单多
		EachMaxShort: 1,   // 最多开1单空
		OnBar: func(s *strat.StratJob) {
			// 如果处于预热期，不进行交易
			if s.IsWarmUp {
				return
			}

			e := s.Env
			// 计算短期和长期SMA
			maShort := ta.SMA(e.Close, smlLen)
			maLong := ta.SMA(e.Close, bigLen)

			// 检测均线交叉
			cross := maShort.Cross(maLong)

			// 金叉：短期均线上穿长期均线
			if cross == 1 {
				// 开多
				log.Info("open order long", zap.Float64("price", s.Env.Close.Get(0)))
				if err_ := s.OpenOrder(&strat.EnterReq{
					Tag:   "long",
					Limit: s.Env.Close.Get(0),
				}); err_ != nil {
					log.Error("open order long error", zap.Error(err_))
				}
				// 平空
				if err_ := s.CloseOrders(&strat.ExitReq{
					Tag:  "exitS",
					Dirt: core.OdDirtShort,
				}); err_ != nil {
					log.Error("close order exitS error", zap.Error(err_))
				}
			}

			// 死叉：短期均线下穿长期均线
			if cross == -1 {
				// 开空
				log.Info("open order short", zap.Float64("price", s.Env.Close.Get(0)))
				if err_ := s.OpenOrder(&strat.EnterReq{
					Tag:   "short",
					Limit: s.Env.Close.Get(0),
					Short: true,
				}); err_ != nil {
					log.Error("open order short error", zap.Error(err_))
				}
				// 平多
				if err_ := s.CloseOrders(&strat.ExitReq{
					Tag:  "exitL",
					Dirt: core.OdDirtLong,
				}); err_ != nil {
					log.Error("close order exitL error", zap.Error(err_))
				}
			}
		},
	}

}
