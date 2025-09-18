package idea

import (
	"fmt"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

/*
如何使用？
	在yaml的run_policy中使用idea:cl即可指定运行当前策略

缠论的主要构件：笔、线段、中枢、走势
笔和线段状态：CLInit/CLValid/CLDone
	最后一个笔可能会被移除，倒数第二个笔有效，倒数第三个笔确认完成
不同品种的不同周期对笔和线段适用不一。
	有些指数行情1分钟上可以用线段，但很多个股5分钟都不适用线段。
线段的完成由下一个线段的开始确认：
	一类：特征序列无缺口，力度小，新线段开始立刻完成当前线段
	二类：特征序列由缺口，力度大，新线段须由顶底分型确认有效，当前线段才完成

任务进度：
	* 笔：和原文略有不同，兼容一根极大K线包含几十根K线的情况
	* 线段：和原文略有不同，向后看最多7笔确定线段完成点
	* 中枢：只按最基本方法实现了
	* 趋势：未实现

*/

func init() {
	strat.AddStratGroup("idea", map[string]strat.FuncMakeStrat{
		"cl": CL01,
	})
}

type CLMore struct {
	Main   *ta.CGraph // 交易周期缠论对象
	Big    *ta.CGraph // 大周期缠论对象
	bSubMa float64    // 大周期最新价减去MA5
	bDirt  float64    // 大周期方向
	bPen   *ta.CPen   // 大周期最新的笔
	pen    *ta.CPen   //交易周期最新的笔
}

func CL01(pol *config.RunPolicyConfig) *strat.TradeStrat {
	bigTf := "2h"
	return &strat.TradeStrat{
		RunTimeFrames: []string{"15m"},
		WarmupNum:     300,
		OnStartUp: func(s *strat.StratJob) {
			m := &CLMore{
				Main: &ta.CGraph{},
				Big:  &ta.CGraph{},
			}
			m.Big.OnPen = func(p *ta.CPen, evt int) {
				if evt != ta.EvtNew {
					return
				}
				m.bPen = p
				if p.Dirt*m.bSubMa > 0 {
					m.bDirt = p.Dirt
				} else {
					m.bDirt = 0
				}
			}
			m.Main.OnPen = func(p *ta.CPen, evt int) {
				if evt != ta.EvtNew {
					return
				}
				m.pen = p
			}
			s.More = m
		},
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				//{"_cur_", smlTf, 30},
				{"_cur_", bigTf, 300},
			}
		},
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			m, _ := s.More.(*CLMore)
			m.Main.AddBar(e)
			m.Main.Parse()
			atr := ta.ATR(e.High, e.Low, e.Close, 14)
			rsi := ta.RSI(e.Close, 14)
			if m.Main.OnPoint == nil {
				m.Main.OnPoint = func(p *ta.CPoint, evt int) {
					if evt != ta.EvtNew {
						return
					}
					var ods []*ormo.InOutOrder
					if p.Dirt < 0 {
						ods = s.LongOrders
					} else if p.Dirt > 0 {
						ods = s.ShortOrders
					} else {
						return
					}
					if len(ods) == 0 {
						if p.Dirt*m.bDirt < 0 && rsi.Get(0) > 60 {
							// 当前周期笔方向与大周期笔方向不一致
							atrVal := atr.Get(0)
							s.OpenOrder(&strat.EnterReq{
								Tag:           fmt.Sprintf("op%v", m.bDirt),
								Short:         m.bDirt < 0,
								StopLossVal:   atrVal * 2.5,
								TakeProfitVal: atrVal * 2.5,
								Log:           true,
							})
						}
					} else {
						s.CloseOrders(&strat.ExitReq{
							Tag:  "exit",
							Dirt: int(m.bDirt),
							Log:  true,
						})
					}
				}
			}
		},
		OnInfoBar: func(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
			m, _ := s.More.(*CLMore)
			if tf == bigTf {
				m.Big.AddBar(e)
				m.Big.Parse()
				ma5 := ta.SMA(e.Close, 5)
				m.bSubMa = e.Close.Get(0) - ma5.Get(0)
			}
		},
	}
}
