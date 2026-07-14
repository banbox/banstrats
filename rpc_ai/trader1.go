package rpc_ai

import (
	"context"
	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	ta "github.com/banbox/banta"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/mat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"math"
	"strings"
	"time"
)

const (
	aiGRPCAddrKey      = "ai_grpc_addr"
	aiGRPCTimeoutMSKey = "ai_grpc_timeout_ms"
	defaultAITimeout   = 5 * time.Second
	maxAITimeout       = time.Minute
)

type AIMore struct {
	feasBig *mat.Dense // 大周期特征
	feas    *mat.Dense // 小周期特征
	info    []float64
	atr     float64
	age     int // 已持仓bar数
}

type aiRPCClient struct {
	client  biz.AInferClient
	conn    *grpc.ClientConn
	timeout time.Duration
}

func newAIRPCClient(pol *config.RunPolicyConfig) (*aiRPCClient, error) {
	addr, timeout := aiRPCConfig(pol)
	if addr == "" {
		return nil, nil
	}
	const maxMsgSize = 100 * 1024 * 1024
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(maxMsgSize),
			grpc.MaxCallRecvMsgSize(maxMsgSize),
		),
	)
	if err != nil {
		return nil, err
	}
	return &aiRPCClient{
		client:  biz.NewAInferClient(conn),
		conn:    conn,
		timeout: timeout,
	}, nil
}

func aiRPCConfig(pol *config.RunPolicyConfig) (string, time.Duration) {
	var more map[string]interface{}
	if pol != nil {
		more = pol.More
	}
	addr, _ := more[aiGRPCAddrKey].(string)
	addr = strings.TrimSpace(addr)
	timeout := defaultAITimeout
	if value, ok := more[aiGRPCTimeoutMSKey]; ok {
		switch value := value.(type) {
		case int:
			timeout = time.Duration(value) * time.Millisecond
		case int64:
			timeout = time.Duration(value) * time.Millisecond
		case float64:
			timeout = time.Duration(value * float64(time.Millisecond))
		}
	}
	if timeout <= 0 || timeout > maxAITimeout {
		timeout = defaultAITimeout
	}
	return addr, timeout
}

func AITrade(pol *config.RunPolicyConfig) *strat.TradeStrat {
	var seqNum = 50        // 特征序列长度
	var maxOdNum = 1.0     // 单币种单方向最多开1单
	const maxHoldAge = 300 // 持仓age超过此配置，强制平仓
	rpcClient, rpcErr := newAIRPCClient(pol)
	return &strat.TradeStrat{
		BatchInOut: true,
		WarmupNum:  300,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &AIMore{}
		},
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{Pair: "_cur_", TimeFrame: "1h", WarmupNum: 300},
			}
		},
		OnBar: func(s *strat.StratJob) {
			m, _ := s.More.(*AIMore)
			e := s.Env
			//if !core.IsWarmUp {
			//	log.Info("OnBar", zap.Int64("time", e.TimeStop))
			//	defer log.Info("OnBarEnd", zap.Int64("time", e.TimeStop))
			//}
			m.feas = onAiFeatures(e, seqNum)
			o, h, l, c, v := e.Open.Get(0), e.High.Get(0), e.Low.Get(0), e.Close.Get(0), e.Volume.Get(0)
			info := []float64{float64(e.TimeStart / 1000), o, h, l, c, v}
			info = append(info, 0, 0, 0)
			m.info = info
			m.atr = ta.ATR(e.High, e.Low, e.Close, 30).Get(0)
		},
		OnInfoBar: func(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
			//if !core.IsWarmUp {
			//	log.Info("OnInfoBar", zap.Int64("time", e.TimeStop))
			//	defer log.Info("OnInfoBarEnd", zap.Int64("time", e.TimeStop))
			//}
			m, _ := s.More.(*AIMore)
			m.feasBig = onAiFeatures(e, seqNum)
		},
		OnBatchJobs: func(jobs []*strat.StratJob) {
			var valids []*strat.StratJob
			var feas1, feas2, info []float64
			var feasLen, feasDepth, infoDepth int
			for _, j := range jobs {
				m, _ := j.More.(*AIMore)
				if m == nil || m.feas == nil || m.feasBig == nil || len(m.info) == 0 {
					continue
				}
				rows, cols := m.feas.Dims()
				bigRows, bigCols := m.feasBig.Dims()
				if rows == 0 || cols == 0 || rows != bigRows || cols != bigCols {
					log.Warn("skip ai inference job with incompatible feature shapes",
						zap.Int("feas_rows", rows), zap.Int("feas_cols", cols),
						zap.Int("feas_big_rows", bigRows), zap.Int("feas_big_cols", bigCols))
					continue
				}
				if len(valids) == 0 {
					feasLen, feasDepth = rows, cols
					infoDepth = len(m.info)
				} else if rows != feasLen || cols != feasDepth || len(m.info) != infoDepth {
					log.Warn("skip ai inference job with inconsistent batch shape",
						zap.Int("feas_rows", rows), zap.Int("feas_cols", cols),
						zap.Int("info_depth", len(m.info)))
					continue
				}
				valids = append(valids, j)
				feas1 = append(feas1, m.feas.RawMatrix().Data...)
				feas2 = append(feas2, m.feasBig.RawMatrix().Data...)
				info = append(info, m.info...)
			}
			//if !core.IsWarmUp {
			//	log.Info("OnBatchJobs", zap.Int64("time", timeStop))
			//	defer log.Info("OnBatchJobs", zap.Int64("time", timeStop))
			//}
			if len(valids) == 0 {
				return
			}
			if rpcErr != nil {
				log.Error("create ai inference client failed", zap.Error(rpcErr))
				return
			}
			if rpcClient == nil {
				log.Warn("skip ai inference: set run_policy.ai_grpc_addr to enable rpc_ai:trade1")
				return
			}
			bSize := len(valids)
			feaShape := []int32{int32(bSize), int32(feasLen), int32(feasDepth)}
			ctx, cancel := context.WithTimeout(context.Background(), rpcClient.timeout)
			defer cancel()

			trend, err_ := rpcClient.client.Trend(ctx, &biz.ArrMap{
				Mats: map[string]*biz.NumArr{
					"feas":  {Data: feas1, Shape: feaShape},
					"feas2": {Data: feas2, Shape: feaShape},
					"info":  {Data: info, Shape: []int32{int32(bSize), int32(infoDepth)}},
				},
			})
			if err_ != nil {
				log.Error("call ai trend failed", zap.Int("bSize", bSize), zap.Error(err_))
				return
			}
			if trend == nil || trend.Mats["pred"] == nil {
				log.Error("ai trend response has no pred matrix", zap.Int("bSize", bSize))
				return
			}
			preds := trend.Mats["pred"].Data
			if len(preds) != bSize {
				log.Error("ai trend prediction count mismatch",
					zap.Int("expected", bSize), zap.Int("actual", len(preds)))
				return
			}
			for i, j := range valids {
				m, _ := j.More.(*AIMore)
				if math.IsNaN(preds[i]) || math.IsInf(preds[i], 0) {
					log.Warn("skip invalid ai prediction", zap.Float64("prediction", preds[i]))
					continue
				}
				pred := int(math.Round(preds[i])) // 1: long  2: short
				truncated := false
				if len(j.LongOrders) > 0 || len(j.ShortOrders) > 0 {
					m.age += 1
					if m.age >= maxHoldAge {
						truncated = true
					}
				}
				if (pred == 2 || truncated) && len(j.LongOrders) > 0 {
					_ = j.CloseOrders(&strat.ExitReq{
						Tag:  "exit_long",
						Dirt: core.OdDirtLong,
					})
				}
				if truncated || pred == 1 && len(j.ShortOrders) > 0 {
					_ = j.CloseOrders(&strat.ExitReq{
						Tag:  "exit_short",
						Dirt: core.OdDirtShort,
					})
				} else if pred == 1 && len(j.LongOrders) < int(maxOdNum) {
					_ = j.OpenOrder(&strat.EnterReq{
						Tag: "long",
					})
				} else if pred == 2 && len(j.ShortOrders) < int(maxOdNum) {
					_ = j.OpenOrder(&strat.EnterReq{
						Tag:   "short",
						Short: true,
					})
				}
			}
		},
		OnShutDown: func(*strat.StratJob) {
			if rpcClient != nil {
				_ = rpcClient.conn.Close()
			}
		},
	}
}
