package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

func init() {
	// 注册策略到banbot中，后续在配置文件中使用ma:demo即可引用此策略
	// `init`函数是go中的特殊函数，会在当前包被导入时立刻执行
	strat.AddStratGroup("ma", map[string]strat.FuncMakeStrat{
		"demo":       Demo,
		"demo_er":    DemoER,
		"demo2":      DemoInfo,
		"demo_batch": BatchDemo,
		"demo_exit":  CustomExitDemo,
		"postApi":    PostApi,
	})
}

func Demo(pol *config.RunPolicyConfig) *strat.TradeStrat {
	smlLen := int(pol.Def("smlLen", 5, core.PNorm(3, 10)))
	bigLen := int(pol.Def("bigLen", 20, core.PNorm(10, 40)))
	return &strat.TradeStrat{
		WarmupNum: 100,
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			ma5 := ta.SMA(e.Close, smlLen)
			ma20 := ta.SMA(e.Close, bigLen)
			maCrx := ta.Cross(ma5, ma20)

			if maCrx == 1 {
				s.OpenOrder(&strat.EnterReq{Tag: "long"})
				if len(s.ShortOrders) > 0 {
					s.CloseOrders(&strat.ExitReq{Tag: "exitS", Dirt: core.OdDirtShort})
				}
			} else if maCrx == -1 {
				s.OpenOrder(&strat.EnterReq{Tag: "short", Short: true})
				if len(s.LongOrders) > 0 {
					s.CloseOrders(&strat.ExitReq{Tag: "exitL", Dirt: core.OdDirtLong})
				}
			}
		},
	}
}
