package ma

import (
	"fmt"
	"strings"

	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	ta "github.com/banbox/banta"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/floats"
)

/*
stamp := btime.UTCStamp() // latest time
e := s.GetTmpEnv(stamp, k.Open, k.High, k.Low, k.Close, k.Volume, k.Quote)
ma5 := ta.SMA(e.Close, 5).Get(0)

GetTmpEnv可返回一个临时的指标计算环境，传入的时间戳应当是此未完成数据的最新截止毫秒时间戳，针对每个品种+周期，仅保留一个临时指标环境，当传入更新的时间戳时，旧的环境会被舍弃。
*/

func taUndo(p *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum: 10,
		WsSubs: map[string]string{
			core.WsSubKLine: "",
		},
		OnStartUp: func(s *strat.StratJob) {
			s.More = [5]float64{}
		},
		OnWsKline: func(s *strat.StratJob, pair string, k *banexg.Kline) {
			e := s.GetTmpEnv(btime.UTCStamp(), k.Open, k.High, k.Low, k.Close, k.Volume, k.Quote, k.BuyVolume, k.TradeNum)
			o1, h1, l1, c1, v1 := e.Open.Get(1), e.High.Get(1), e.Low.Get(1), e.Close.Get(1), e.Volume.Get(1)
			h0, l0, c0, v0 := e.High.Get(0), e.Low.Get(0), e.Close.Get(0), e.Volume.Get(0)
			pa := s.More.([5]float64)
			fails := make([]string, 0, 2)
			if pa[0] != o1 {
				fails = append(fails, "o1")
			}
			if pa[1] != h1 {
				fails = append(fails, "h1")
			}
			if pa[2] != l1 {
				fails = append(fails, "l1")
			}
			if pa[3] != c1 {
				fails = append(fails, fmt.Sprintf("c1:%v:%v", pa[3], c1))
			}
			if pa[4] != v1 {
				fails = append(fails, fmt.Sprintf("v1:%v:%v", pa[4], v1))
			}
			if h0 != k.High {
				fails = append(fails, fmt.Sprintf("h0:%v:%v", h0, k.High))
			}
			if l0 != k.Low {
				fails = append(fails, fmt.Sprintf("l0:%v:%v", l0, k.Low))
			}
			if c0 != k.Close {
				fails = append(fails, fmt.Sprintf("c0:%v:%v", c0, k.Close))
			}
			if v0 != k.Volume {
				fails = append(fails, "v0")
			}
			pSum5 := ta.Sum(e.Close, 6).Get(0) - k.Close
			realSum := floats.Sum(e.Close.Range(1, 6))
			if len(fails) > 0 {
				log.Warn("OnWsKline fail", zap.Int("num", len(fails)), zap.Int64("stamp", e.TimeStop),
					zap.String("detail", strings.Join(fails, ",")), zap.Float64("sum5", pSum5),
					zap.Float64("reaSum", realSum),
					zap.Float64s("cls", e.Close.Range(0, 6)))
			} else {
				log.Info("OnWsKline pass", zap.Int64("stamp", e.TimeStop), zap.Float64("o", k.Open),
					zap.Float64("h", k.High), zap.Float64("l", k.Low), zap.Float64("c", k.Close),
					zap.Float64("v", k.Volume), zap.Float64("sum5", pSum5), zap.Float64("reaSum", realSum),
					zap.Float64s("cls", e.Close.Range(0, 6)))
			}
		},
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			s.More = [5]float64{
				e.Open.Get(0),
				e.High.Get(0),
				e.Low.Get(0),
				e.Close.Get(0),
				e.Volume.Get(0),
			}
			sum5 := ta.Sum(e.Close, 5).Get(0)
			log.Info("OnBar", zap.Float64("sum5", sum5), zap.Float64s("cls", e.Close.Range(0, 5)))
		},
	}
}
