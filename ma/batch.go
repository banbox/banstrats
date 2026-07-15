package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banbot/utils"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

func calcCorrs(jobs []*strat.StratJob, isBig bool) {
	if len(jobs) < 2 {
		return
	}
	dataArr := make([][]float64, 0, len(jobs))
	for _, j := range jobs {
		dataArr = append(dataArr, j.Env.Close.Range(0, 70))
	}
	_, arr, err := utils.CalcCorrMat(70, dataArr, true)
	if err != nil {
		log.Error("calc corr mat fail", zap.Error(err))
		return
	}
	for i, j := range jobs {
		m, _ := j.More.(*BatchSta)
		if isBig {
			m.bigCorr = arr[i]
			m.bigCorrReady = true
		} else {
			m.smlCorr = arr[i]
			m.smlCorrReady = true
		}
	}
}

type BatchSta struct {
	smlCorr      float64
	bigCorr      float64
	smlCorrReady bool
	bigCorrReady bool
}

func BatchDemo(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:  100,
		BatchInOut: true,
		BatchInfo:  true,
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{Pair: "_cur_", TimeFrame: "1h", WarmupNum: 100},
			}
		},
		OnStartUp: func(s *strat.StratJob) {
			s.More = &BatchSta{}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				m, _ := s.More.(*BatchSta)
				if m == nil || !m.smlCorrReady || !m.bigCorrReady {
					return
				}
				if m.bigCorr < 0.5 && m.smlCorr < 0.5 {
					// 当大小周期的相关度均低于50%时开单。
					s.OpenOrder(&strat.EnterReq{Tag: "open"})
				} else if m.smlCorr > 0.9 {
					// 当前品种小周期相关度高于90%，平仓
					s.CloseOrders(&strat.ExitReq{Tag: "close"})
				}
			},
		}),
		OnBatchJobs: func(jobs []*strat.StratJob) {
			if len(jobs) < 2 || jobs[0].IsWarmUp {
				return
			}
			calcCorrs(jobs, false)
		},
		OnBatchInfos: func(tf string, jobs map[string]*strat.JobEnv) {
			jobList := make([]*strat.StratJob, 0, len(jobs))
			for _, job := range jobs {
				jobList = append(jobList, job.Job)
			}
			if len(jobList) < 2 || jobList[0].IsWarmUp {
				return
			}
			calcCorrs(jobList, true)
		},
	}
}
