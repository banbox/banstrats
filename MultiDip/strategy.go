package newstrategy4

import (
	"math"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

func NewStrategy4(pol *config.RunPolicyConfig) *strat.TradeStrat {
	p := loadParams(pol)

	return &strat.TradeStrat{
		Name:      "MultiDip",
		Version:   3,
		WarmupNum: 200,
		Policy:    pol,

		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{"_cur_", "1h", 168},
				{"BTC/USDT", "15m", 30},
				{"BTC/USDT", "1h", 30},
			}
		},

		OnStartUp: func(s *strat.StratJob) {
			s.More = &New4More{}
		},

		OnInfoBar: func(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
			onInfoBar1h(s, e, pair, tf)
		},

		OnBar: func(s *strat.StratJob) {
			e := s.Env
			m, _ := s.More.(*New4More)

			closeNow := e.Close.Get(0)
			openNow := e.Open.Get(0)

			rsi14 := ta.RSI(e.Close, 14).Get(0)
			rsiFast := ta.RSI(e.Close, 4).Get(0)
			rsiSlow := ta.RSI(e.Close, 20).Get(0)
			_ = rsiFast
			_ = rsiSlow

			// EWO
			ewoVal := ewoFromEnv(e, p.FastEwo, p.SlowEwo)

			// VWAP lower approx
			vwapLow := vwapLowerFromEnv(e, 20)

			// BBANDS
			upper, mid, lower := ta.BBANDS(e.Close, 20, 2, 2)
			bbWidth := (upper.Get(0) - lower.Get(0)) / math.Max(1e-12, mid.Get(0))

			// amplitude filter
			ampFlag := amplitudeFlagFromEnv(e, 30, 9.5)

			// RMI / CCI / StochRSI / CTI
			rmi := ta.RMI(e.Close, int(pol.Param("buy_rmi_length", 17)), 4).Get(0)
			cci := ta.CCI(e.Close, int(pol.Param("buy_cci_length", 25))).Get(0)
			srsiK := stochRSIValue(e, 15, 20)
			cti := ta.CTI(e.Close, 20).Get(0)

			isDIP := false
			isBreak := false
			isClucHA := false
			isNFIX39 := false
			isNFIX29 := false
			isLocalUptrend := false
			isVwap := false
			isInstaSignal := false
			isNFINext44 := false
			isNFINext37 := false
			isNFINext7 := false

			if !ampFlag &&
				rmi < float64(pol.Param("buy_rmi", 49)) &&
				cci <= float64(pol.Param("buy_cci", -116)) &&
				srsiK < float64(pol.Param("buy_srsi_fk", 32)) &&
				(bbWidth > p.BuyBbWidth && bbWidth > p.BuyBbDelta) &&
				(closeNow < lower.Get(0)*p.BuyBbFactor) &&
				(m.Roc1h < p.BuyRoc1h) {
				isDIP = true
			}

			if !ampFlag &&
				(bbWidth > p.BuyBbWidth && bbWidth > p.BuyBbDelta) &&
				(closeNow < lower.Get(0)*p.BuyBbFactor) &&
				(m.Roc1h < p.BuyRoc1h) {
				isBreak = true
			}

			if !ampFlag && rocrFromEnv(e, 28) > float64(pol.Param("buy_clucha_rocr_1h", 0.526)) {
				isClucHA = true
			}

			if !ampFlag && (ta.EMA(e.Close, 200).Get(0) > ta.EMA(e.Close, 200).Get(12)*1.01) {
				isNFIX39 = true
			}

			if !ampFlag &&
				(closeNow > m.SupLevel1h*0.72) &&
				(closeNow < ta.EMA(e.Close, 16).Get(0)*0.982) &&
				(ewoVal < -10.0) &&
				(cti < -0.9) {
				isNFIX29 = true
			}

			if !ampFlag &&
				ta.EMA(e.Close, 26).Get(0) > ta.EMA(e.Close, 12).Get(0) &&
				(ta.EMA(e.Close, 26).Get(0)-ta.EMA(e.Close, 12).Get(0) > openNow*p.BuyClosedelta/1000.0) &&
				(closeNow < lower.Get(0)*p.BuyBbFactor) {
				isLocalUptrend = true
			}

			if !ampFlag && (closeNow < vwapLow) && (cti < -0.8) && (rsi14 < 35) {
				isVwap = true
			}

			if !ampFlag && (williamsRFromEnv(e, 14) < -51) {
				isInstaSignal = true
			}

			if !ampFlag &&
				(closeNow < ta.EMA(e.Close, 16).Get(0)*0.982) &&
				(ewoVal < -18.0) {
				isNFINext44 = true
			}
			if !ampFlag &&
				(ta.EMA(e.Close, 26).Get(0) > ta.EMA(e.Close, 12).Get(0)) &&
				((ta.EMA(e.Close, 26).Get(0)-ta.EMA(e.Close, 12).Get(0)) > (openNow * 0.03)) {
				isNFINext7 = true
			}
			pmaxVal := pmaxApprox(e, 9, 27, 10)
			if !ampFlag && pmaxVal > ta.SMA(e.Close, 75).Get(0)*0.98 && (ewoVal > 9.8) {
				isNFINext37 = true
			}

			if isDIP { s.OpenOrder(&strat.EnterReq{Tag: "DIP"}) }
			if isBreak { s.OpenOrder(&strat.EnterReq{Tag: "Break"}) }
			if isClucHA { s.OpenOrder(&strat.EnterReq{Tag: "cluc_HA"}) }
			if isNFIX39 { s.OpenOrder(&strat.EnterReq{Tag: "NFIX39"}) }
			if isNFIX29 { s.OpenOrder(&strat.EnterReq{Tag: "NFIX29"}) }
			if isLocalUptrend { s.OpenOrder(&strat.EnterReq{Tag: "local_uptrend"}) }
			if isVwap { s.OpenOrder(&strat.EnterReq{Tag: "vwap"}) }
			if isInstaSignal { s.OpenOrder(&strat.EnterReq{Tag: "insta_signal"}) }
			if isNFINext44 { s.OpenOrder(&strat.EnterReq{Tag: "NFINext44"}) }
			if isNFINext37 { s.OpenOrder(&strat.EnterReq{Tag: "NFINext37"}) }
			if isNFINext7 { s.OpenOrder(&strat.EnterReq{Tag: "NFINext7"}) }

			if fisherFromEnv(e, 10) > p.SellFisher && (e.High.Get(0) <= ta.EMA(e.Close, 3).Get(0)) {
				s.CloseOrders(&strat.ExitReq{Tag: "sell"})
			}
		},

		OnCheckExit: func(s *strat.StratJob, od *ormo.InOutOrder) *strat.ExitReq {
			if od == nil {
				return nil
			}
			if od.ProfitRate > 0.1 {
				return &strat.ExitReq{Tag: "profit"}
			}
			if od.ProfitRate < float64(pol.Param("sell_deadfish_profit", -0.08)) {
				return &strat.ExitReq{Tag: "sell_stoploss_deadfish"}
			}
			return nil
		},

		OnOrderChange: func(s *strat.StratJob, od *ormo.InOutOrder, chgType int) {
			if od == nil {
				return
			}
			longOrders := s.GetOrders(core.OdDirtLong)
			count := len(longOrders)
			if count >= 1 && count <= p.MaxSafetyOrders {
				trigger := math.Abs(float64(p.InitialSafetyTrigger)) * float64(count)
				if od.ProfitRate <= -trigger {
					costRate := math.Pow(float64(p.SafetyOrderVolumeScale), float64(count-1))
					s.OpenOrder(&strat.EnterReq{Tag: "safety_order", CostRate: costRate})
				}
			}
		},
	}
} 