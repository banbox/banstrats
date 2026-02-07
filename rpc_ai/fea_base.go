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
	onBar := func(bar *orm.InfoKline, nexts []*orm.InfoKline) {
		state := states[bar.Symbol]
		var err *errs.Error
		if bar.TimeFrame == task.TfSml {
			futs := make([]*banexg.Kline, 0, len(nexts))
			for _, n := range nexts {
				futs = append(futs, &n.Kline)
			}
			state.env.OnBar(bar.Time, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Quote, bar.BuyVolume, bar.TradeNum)
			err = task.OnBar(state.env, &bar.Kline, futs)
		} else {
			state.envBig.OnBar(bar.Time, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Quote, bar.BuyVolume, bar.TradeNum)
			err = task.OnInfoBar(state.envBig, &bar.Kline)
		}
		if err != nil {
			outErr = err
			verCh <- -1
		}
	}
	err := biz.RunHistKline(&biz.RunHistArgs{
		ExsList:     exsList,
		Start:       req.Start,
		End:         req.End,
		ViewNextNum: task.NextObsNum,
		TfWarms: map[string]int{
			task.TfSml: task.WarmSml,
			task.TfBig: task.WarmBig,
		},
		OnEnvEnd: func(bar *banexg.PairTFKline, adj *orm.AdjInfo) {
			if bar != nil {
				state := states[bar.Symbol]
				state.env.Reset()
				state.envBig.Reset()
			}
			task.OnEnvEnd(bar, adj)
		},
		VerCh: verCh,
		OnBar: onBar,
	})
	if err != nil {
		return err
	}
	if outErr != nil {
		return outErr
	}
	return nil
}
