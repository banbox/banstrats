package rpc_ai

import (
	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banta"
)

type GenFeaTask struct {
	TfSml      string
	TfBig      string
	WarmSml    int
	WarmBig    int
	TfMSecs    int64
	TfMSecsBig int64
	NextObsNum int
	OnBar      func(e *banta.BarEnv, b *banexg.Kline, futs []*banexg.Kline) *errs.Error
	OnInfoBar  func(e *banta.BarEnv, b *banexg.Kline) *errs.Error
	OnEnvEnd   func(bar *banexg.PairTFKline, adj *orm.AdjInfo)
}

type CodeState struct {
	env    *banta.BarEnv
	envBig *banta.BarEnv
}

func pubFeaBase(exsList []*orm.ExSymbol, req *biz.SubReq, task *GenFeaTask) error {
	var states = make(map[string]*CodeState)
	for _, exs := range exsList {
		states[exs.Symbol] = &CodeState{
			env: &banta.BarEnv{
				TimeFrame:  task.TfSml,
				TFMSecs:    task.TfMSecs,
				Exchange:   exs.Exchange,
				MarketType: exs.Market,
				Symbol:     exs.Symbol,
				MaxCache:   1000,
			},
			envBig: &banta.BarEnv{
				TimeFrame:  task.TfBig,
				TFMSecs:    task.TfMSecsBig,
				Exchange:   exs.Exchange,
				MarketType: exs.Market,
				Symbol:     exs.Symbol,
				MaxCache:   500,
			},
		}
	}
	var outErr *errs.Error
	verCh := make(chan int, 5)
	onData := func(evt *orm.DataSeries, nexts []*orm.DataSeries) {
		if evt == nil {
			return
		}
		view, errView := evt.OHLCV(evt.ExSymbol)
		if errView != nil {
			outErr = errs.New(errs.CodeRunTime, errView)
			verCh <- -1
			return
		}
		state := states[view.Symbol()]
		var err *errs.Error
		if view.TimeFrame == task.TfSml {
			futs := make([]*banexg.Kline, 0, len(nexts))
			for _, n := range nexts {
				nextView, errView := n.OHLCV(n.ExSymbol)
				if errView != nil {
					outErr = errs.New(errs.CodeRunTime, errView)
					verCh <- -1
					return
				}
				if bar := nextView.Bar(); bar != nil {
					futs = append(futs, bar)
				}
			}
			state.env.OnBar(view.Time, view.Open, view.High, view.Low, view.Close, view.Volume, view.Quote, view.BuyVolume, view.TradeNum)
			bar := view.Bar()
			err = task.OnBar(state.env, bar, futs)
		} else {
			state.envBig.OnBar(view.Time, view.Open, view.High, view.Low, view.Close, view.Volume, view.Quote, view.BuyVolume, view.TradeNum)
			err = task.OnInfoBar(state.envBig, view.Bar())
		}
		if err != nil {
			outErr = err
			verCh <- -1
		}
	}
	err := biz.RunHistSeries(&biz.RunHistSeriesArgs{
		ExsList:     exsList,
		Start:       req.Start,
		End:         req.End,
		ViewNextNum: task.NextObsNum,
		TfWarms: map[string]int{
			task.TfSml: task.WarmSml,
			task.TfBig: task.WarmBig,
		},
		OnEnvEnd: func(evt *orm.DataSeries) {
			if evt != nil {
				state := states[evt.Symbol()]
				state.env.Reset()
				state.envBig.Reset()
			}
			if task.OnEnvEnd != nil {
				var pairTF *banexg.PairTFKline
				var adj *orm.AdjInfo
				if evt != nil {
					if view, errView := evt.OHLCV(evt.ExSymbol); errView == nil {
						if bar := view.Bar(); bar != nil {
							pairTF = &banexg.PairTFKline{Symbol: view.Symbol(), TimeFrame: view.TimeFrame, Kline: *bar}
						}
						adj = view.Adj
					}
				}
				task.OnEnvEnd(pairTF, adj)
			}
		},
		VerCh:  verCh,
		OnData: onData,
	})
	if err != nil {
		return err
	}
	if outErr != nil {
		return outErr
	}
	return nil
}
