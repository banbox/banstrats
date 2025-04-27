package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banbot/utils"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

func calcCorrs(jobs []*strat.StratJob, isBig bool) {
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
		} else {
			m.smlCorr = arr[i]
		}
	}
}

type BatchSta struct {
	smlCorr float64
	bigCorr float64
}

func BatchDemo(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:  100,
		BatchInOut: true,
		BatchInfo:  true,
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{"_cur_", "1h", 100},
			}
		},
		OnStartUp: func(s *strat.StratJob) {
			s.More = &BatchSta{}
		},
		OnBar: func(s *strat.StratJob) {
			m, _ := s.More.(*BatchSta)
			if m.bigCorr < 0.5 && m.smlCorr < 0.5 {
				// 当大小周期的相关度均低于50%时开单。
				s.OpenOrder(&strat.EnterReq{Tag: "open"})
			} else if m.smlCorr > 0.9 {
				// 当前品种小周期相关度高于90%，平仓
				s.CloseOrders(&strat.ExitReq{Tag: "close"})
			}
		},
		OnBatchJobs: func(jobs []*strat.StratJob) {
			if jobs[0].IsWarmUp {
				return
			}
			calcCorrs(jobs, false)
		},
		OnBatchInfos: func(tf string, jobs map[string]*strat.JobEnv) {
			jobList := make([]*strat.StratJob, 0, len(jobs))
			for _, job := range jobs {
				jobList = append(jobList, job.Job)
			}
			if jobList[0].IsWarmUp {
				return
			}
			calcCorrs(jobList, true)
		},
	}
}
