package grid

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banbot/utils"
	ta "github.com/banbox/banta"
)

/*
CheckPos checks the position on each bar's close to see if replenishment or take profit is needed.
Ignore the high/low in the middle of the bar. So try to execute within a 1m cycle as much as possible to avoid the problem of a bar's high/low breaking through the grid and the close being retrieved, resulting in unexecuted execution
CheckPos在每个bar的close上检查位置，是否需要补仓或止盈。
bar中间的high/low忽略。所以尽量在1m周期执行，避免一个bar的high/low突破网格，close又收回来导致未执行的问题
*/

func init() {
	strat.AddStratGroup("grid", map[string]strat.FuncMakeStrat{
		"inv": InvGrid,
	})
}

func NewGrid(initRate, maxAdd int, slRate float64, debug bool) *Grid {
	return &Grid{
		InitRate: float64(initRate),
		MaxAdd:   maxAdd,
		SLRate:   slRate,
		Debug:    debug,
	}
}

type Grid struct {
	InitRate float64 // Opening the grid for the first time to input multiple 初次打开网格投入倍数
	MaxAdd   int     // Maximum number of warehouse additions 最大加仓次数
	SLRate   float64 // Multiple of stop loss relative to Unit 止损相对Unit的倍数
	Dirt     float64 // Are you currently long or short on the grid; 1 long, -1 short 当前是做多网格还是做空；1做多，-1做空
	HoldSize float64 // Holding position ratio; Relative unit opening size 持有仓位倍率；相对单位开仓大小
	Unit     float64 // grid price unit 网格价格单位
	OneAmt   float64 // The quantity is calculated upon initial entry 一份的数量，初次入场时计算
	EntPrice float64 // Entry price 入场价格
	Pos      float64 // grid position, integer, 网格位置，整数，-6/-5/-4/-3/-2/-1/0/1/2/3/4/5/6
	OldPos   float64 // Last grid location 上次网格位置
	openNeed bool    // whether to open the warehouse receipt 是否需要打开加仓单
	takeNeed bool    // whether to add a profit taking order 是否需要追加止盈单

	Debug   bool
	barLogs []string
	barMS   int64
}

func (m *Grid) UpdatePos(e *ta.BarEnv, price float64) {
	offRaw := (price - m.EntPrice) / m.Unit
	rate := math.Floor(math.Abs(offRaw) + 0.1)
	if offRaw < 0 {
		rate = -rate
	}
	if rate != m.Pos {
		m.Pos = rate
		m.takeNeed = true
		m.openNeed = true
		if m.Debug {
			base := m.EntPrice + m.Pos*m.Unit
			m.log(e, "upper: %.2f  lower: %.2f", base+m.Unit, base-m.Unit)
		}
	}
}

func (m *Grid) Open(s *strat.StratJob) {
	c := s.Env.Close.Get(0)
	var tag = "short"
	if m.Dirt > 0 {
		tag = "long"
	}
	m.EntPrice = 0
	m.Pos = 0
	m.OldPos = 0
	m.HoldSize = 0
	m.OneAmt = s.Strat.GetStakeAmount(s) / c
	m.openNeed = true
	m.takeNeed = true
	s.OpenOrder(&strat.EnterReq{
		Tag:         tag,
		Amount:      m.InitRate * m.OneAmt,
		Short:       m.Dirt < 0,
		StopLossVal: m.Unit * m.SLRate,
	})
	if m.Debug {
		m.log(s.Env, "init open, dir: %v, size: %v, grid: %.3f, pct: %.1f",
			m.Dirt, m.InitRate, m.Unit, m.Unit*100/c)
	}
}

func (m *Grid) CheckPos(s *strat.StratJob) {
	e := s.Env
	m.UpdatePos(e, e.Close.Get(0))
	if !m.openNeed && !m.takeNeed {
		return
	}
	odList := s.GetOrders(m.Dirt)
	// Cancel trigger orders that have not yet entered the venue
	// 取消未入场的触发订单
	cancels := make([]string, 0)
	var holds = make(map[string]int)
	for _, od := range odList {
		if od.Status < ormo.InOutStatusPartEnter {
			s.CloseOrders(&strat.ExitReq{Tag: "cancel", OrderID: od.ID, Force: true})
			if m.Debug {
				cancels = append(cancels, strconv.FormatFloat(od.Enter.Price, 'f', 4, 64))
			}
		} else if m.Debug {
			num := int(math.Round(od.HoldAmount() / m.OneAmt))
			row := strconv.FormatFloat(od.Enter.Average, 'f', 2, 64)
			old, _ := holds[row]
			holds[row] = old + num
		}
	}
	if m.HoldSize < 0.1 {
		if m.Debug {
			m.log(e, "hold reset\n")
		}
		s.CloseOrders(&strat.ExitReq{Tag: "clear", Force: true})
		return
	}
	posChg := (m.Pos - m.OldPos) * m.Dirt
	m.OldPos = m.Pos
	jumpNum := math.Abs(posChg) - 1
	var jumpStr string
	if jumpNum > 0 {
		// The market has undergone drastic changes, and the latest prices have shifted multiple grids at this time
		// 市场剧烈变化，此时最新价格已偏移多个网格
		extAmt := m.OneAmt * jumpNum
		tag := "jadd"
		if posChg > 0 {
			// Vigorously profitable, selling additional positions
			// 剧烈盈利，卖出额外持仓
			tag = "jexit"
			s.CloseOrders(&strat.ExitReq{
				Tag:        tag,
				Amount:     extAmt,
				FilledOnly: true,
				Force:      true,
			})
		} else {
			// Severe losses, buy additional positions
			// 剧烈亏损，买入额外持仓
			s.OpenOrder(&strat.EnterReq{
				Tag:      tag,
				Amount:   extAmt,
				Short:    m.Dirt < 0,
				StopLoss: odList[0].GetStopLoss().Price,
			})
		}
		if m.Debug {
			jumpStr = fmt.Sprintf("jump %v %.0f ", tag, jumpNum)
		}
	}
	// Set the latest take profit and stop loss waiting to be triggered
	// 设置最新的止盈止损等待触发
	trigPrice := m.EntPrice + m.Pos*m.Unit
	openPrice, closePrice := trigPrice-m.Unit, trigPrice+m.Unit
	if m.Dirt < 0 {
		openPrice, closePrice = closePrice, openPrice
	}
	var openInfo string
	if m.openNeed {
		if -m.Pos*m.Dirt >= float64(m.MaxAdd) {
			// 达到最大加仓深度
			if m.Debug {
				openInfo = "open=nil "
			}
		} else {
			s.OpenOrder(&strat.EnterReq{
				Tag:      "add",
				Limit:    openPrice,
				Amount:   m.OneAmt,
				Short:    m.Dirt < 0,
				StopLoss: odList[0].GetStopLoss().Price,
			})
			if m.Debug {
				openInfo = fmt.Sprintf("open=%.4f ", openPrice)
			}
		}
	}
	m.openNeed = false
	if m.takeNeed {
		s.SetAllTakeProfit(m.Dirt, &ormo.ExitTrigger{
			Price: closePrice,
			Rate:  1 / m.HoldSize,
			Tag:   "exit",
		})
	}
	m.takeNeed = false
	if m.Debug {
		var takes = make(map[string]int)
		for _, od := range odList {
			tp := od.GetTakeProfit()
			if tp != nil {
				tpNum := math.Round(od.HoldAmount() / m.OneAmt)
				if tp.Rate > 0 && tp.Rate < 1 {
					tpNum *= tp.Rate
				}
				tpPrice := strconv.FormatFloat(tp.Price, 'f', 2, 64)
				old, _ := takes[tpPrice]
				takes[tpPrice] = old + int(math.Round(tpNum))
			}
		}
		var cancel string
		if len(cancels) > 0 {
			cancel = "cancel: [" + strings.Join(cancels, ", ") + "] "
		}
		hold := printAmts(holds, "hold: ")
		take := printAmts(takes, "take: ")
		m.log(e, "%s", cancel+jumpStr+openInfo+hold+take)
	}
}

func (m *Grid) OnOrderChange(s *strat.StratJob, od *ormo.InOutOrder, chgType int) {
	if chgType == strat.OdChgEnterFill && od.Enter.Filled > core.AmtDust {
		// Completion of warehouse entry event
		// 加仓入场完成事件
		e := s.Env
		if m.EntPrice == 0 {
			// Set the first downside buy and the first profitable sell
			// 设置第一个下跌买入和第一个盈利卖出
			m.EntPrice = od.Enter.Average
			stoploss := od.GetStopLoss().Price
			upgaps := make([]string, 0, 5)
			dngaps := make([]string, 0, 5)
			for i := 0; i < 5; i++ {
				dist := m.Unit * float64(i+1)
				upgaps = append(upgaps, strconv.FormatFloat(m.EntPrice+dist, 'f', 4, 64))
				dngaps = append(dngaps, strconv.FormatFloat(m.EntPrice-dist, 'f', 4, 64))
			}
			if m.Debug {
				m.log(e, "p=%.4f sl=%.4f", m.EntPrice, stoploss)
				m.log(e, "up: %v", strings.Join(upgaps, " "))
				m.log(e, "dn: %v", strings.Join(dngaps, " "))
			}
		}
		addNum := math.Round(od.Enter.Filled / m.OneAmt)
		m.HoldSize += addNum
		m.openNeed = addNum > 0
		if m.Debug {
			m.log(e, "[enter] %.4f %.0f hold %.0f",
				od.Enter.Average, addNum, m.HoldSize)
		}
	} else if chgType == strat.OdChgExitFill && od.Exit.Filled > core.AmtDust {
		e := s.Env
		exitNum := math.Round(od.Exit.Filled / m.OneAmt)
		m.HoldSize -= exitNum
		m.takeNeed = exitNum > 0
		if m.Debug {
			m.log(e, "[exit] %v %.4f -> %.4f amt %.0f hold %.0f",
				od.ExitTag, od.Enter.Average, od.Exit.Average, exitNum, m.HoldSize)
		}
	}
}

func (m *Grid) log(e *ta.BarEnv, tpl string, args ...any) {
	if e.TimeStart > m.barMS {
		content := strings.Join(m.barLogs, "\n\t")
		fmt.Print(content)
		fmt.Println()
		date := btime.ToDateStr(e.TimeStart, core.DefaultDateFmt)
		m.barLogs = []string{date + fmt.Sprintf(" h: %.2f l: %.2f c: %.2f",
			e.High.Get(0), e.Low.Get(0), e.Close.Get(0))}
		m.barMS = e.TimeStart
	}
	m.barLogs = append(m.barLogs, fmt.Sprintf(tpl, args...))
}

func printAmts(amts map[string]int, prefix string) string {
	if len(amts) == 0 {
		return ""
	}
	ents := utils.KeysOfMap(amts)
	slices.Sort(ents)
	for i, k := range ents {
		if amts[k] > 1 {
			ents[i] += " * " + strconv.Itoa(amts[k])
		}
	}
	return prefix + "[" + strings.Join(ents, ", ") + "] "
}
