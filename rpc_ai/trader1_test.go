package rpc_ai

import (
	"context"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
	"google.golang.org/grpc"
)

type aiInferTestServer struct {
	biz.UnimplementedAInferServer

	mu          sync.Mutex
	predictions [][]float64
	requests    []*biz.ArrMap
	delay       bool
	hadDeadline bool
}

func (s *aiInferTestServer) Trend(ctx context.Context, req *biz.ArrMap) (*biz.ArrMap, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	_, s.hadDeadline = ctx.Deadline()
	delay := s.delay
	var preds []float64
	if len(s.predictions) > 0 {
		preds = append([]float64(nil), s.predictions[0]...)
		s.predictions = s.predictions[1:]
	}
	s.mu.Unlock()

	if delay {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &biz.ArrMap{Mats: map[string]*biz.NumArr{
		"pred": {Data: preds, Shape: []int32{int32(len(preds))}},
	}}, nil
}

func (s *aiInferTestServer) snapshot() ([]*biz.ArrMap, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*biz.ArrMap(nil), s.requests...), s.hadDeadline
}

func startAIInferTestServer(t *testing.T, server *aiInferTestServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	biz.RegisterAInferServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() { grpcServer.Stop() })
	return lis.Addr().String()
}

func newAITradeTestJob(t *testing.T, stgy *strat.TradeStrat) *strat.StratJob {
	t.Helper()
	small, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "15m")
	if err != nil {
		t.Fatalf("new primary bar env: %v", err)
	}
	big, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "1h")
	if err != nil {
		t.Fatalf("new info bar env: %v", err)
	}
	const startMS = int64(1704067200000)
	for i := 0; i < 360; i++ {
		base := 100 + float64(i)*0.05
		small.OnBar(startMS+int64(i)*15*60*1000, base-0.2, base+0.8, base-0.7, base, 1000, 0, 0, 0)
		big.OnBar(startMS+int64(i)*60*60*1000, base-0.2, base+0.8, base-0.7, base, 1000, 0, 0, 0)
	}
	job := &strat.StratJob{
		Strat:        stgy,
		Env:          small,
		Symbol:       &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTC/USDT:USDT"},
		TimeFrame:    "15m",
		MaxOpenLong:  0,
		MaxOpenShort: 0,
		CloseLong:    true,
		CloseShort:   true,
	}
	stgy.OnStartUp(job)
	stgy.OnInfoBar(job, big, job.Symbol.Symbol, "1h")
	stgy.OnBar(job)
	return job
}

func TestAITradeUsesConfiguredGRPCForOpenAndCloseSignals(t *testing.T) {
	configureAITradeTest(t)
	server := &aiInferTestServer{predictions: [][]float64{{1}, {2}}}
	stgy := AITrade(&config.RunPolicyConfig{More: map[string]interface{}{
		aiGRPCAddrKey:      startAIInferTestServer(t, server),
		aiGRPCTimeoutMSKey: float64(300),
	}})
	stgy.Name = "rpc_ai:trade1"
	t.Cleanup(func() { stgy.OnShutDown(nil) })

	job := newAITradeTestJob(t, stgy)
	stgy.OnBatchJobs([]*strat.StratJob{job})
	if len(job.Entrys) != 1 || job.Entrys[0].Tag != "long" || job.Entrys[0].Short {
		t.Fatalf("long prediction did not open a long order: %+v", job.Entrys)
	}

	requests, hadDeadline := server.snapshot()
	if !hadDeadline || len(requests) != 1 {
		t.Fatalf("expected one deadline-bound RPC, got calls=%d deadline=%v", len(requests), hadDeadline)
	}
	if got, want := requests[0].Mats["feas"].Shape, []int32{1, 50, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("feature shape = %v, want %v", got, want)
	}
	if got, want := requests[0].Mats["feas2"].Shape, []int32{1, 50, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("info feature shape = %v, want %v", got, want)
	}
	if got, want := requests[0].Mats["info"].Shape, []int32{1, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("info shape = %v, want %v", got, want)
	}

	job.LongOrders = []*ormo.InOutOrder{{IOrder: &ormo.IOrder{Short: false}}}
	stgy.OnBatchJobs([]*strat.StratJob{job})
	if len(job.Exits) != 1 || job.Exits[0].Tag != "exit_long" || job.Exits[0].Dirt != core.OdDirtLong {
		t.Fatalf("short prediction did not close the long order: %+v", job.Exits)
	}
	if len(job.Entrys) != 2 || !job.Entrys[1].Short || job.Entrys[1].Tag != "short" {
		t.Fatalf("short prediction did not open a short order: %+v", job.Entrys)
	}
}

func TestAITradeSkipsUnreadyFeaturesAndTimesOutInference(t *testing.T) {
	configureAITradeTest(t)
	server := &aiInferTestServer{delay: true}
	stgy := AITrade(&config.RunPolicyConfig{More: map[string]interface{}{
		aiGRPCAddrKey:      startAIInferTestServer(t, server),
		aiGRPCTimeoutMSKey: 20,
	}})
	stgy.Name = "rpc_ai:trade1"
	t.Cleanup(func() { stgy.OnShutDown(nil) })

	job := newAITradeTestJob(t, stgy)
	job.More.(*AIMore).feasBig = nil
	stgy.OnBatchJobs([]*strat.StratJob{job})
	if requests, _ := server.snapshot(); len(requests) != 0 {
		t.Fatalf("unready feature job unexpectedly called RPC %d times", len(requests))
	}

	stgy.OnInfoBar(job, job.Env, job.Symbol.Symbol, "1h")
	start := time.Now()
	stgy.OnBatchJobs([]*strat.StratJob{job})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("inference ignored configured timeout: %v", elapsed)
	}
	if len(job.Entrys) != 0 || len(job.Exits) != 0 {
		t.Fatalf("timed out inference mutated orders: entries=%+v exits=%+v", job.Entrys, job.Exits)
	}
	requests, hadDeadline := server.snapshot()
	if len(requests) != 1 || !hadDeadline {
		t.Fatalf("expected one deadline-bound timed RPC, got calls=%d deadline=%v", len(requests), hadDeadline)
	}
}

func configureAITradeTest(t *testing.T) {
	t.Helper()
	oldStake, oldOpenVolRate := config.StakeAmount, config.OpenVolRate
	config.StakeAmount = 100
	config.OpenVolRate = 1
	t.Cleanup(func() {
		config.StakeAmount = oldStake
		config.OpenVolRate = oldOpenVolRate
	})
}
