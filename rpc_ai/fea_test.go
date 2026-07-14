package rpc_ai

import (
	"context"
	"fmt"
	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/mat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"math"
	"os"
	"strings"
	"testing"
)

func TestDataClient(t *testing.T) {
	// 此测试函数连接grpc，输出收到的每个bar的概要信息
	addr := os.Getenv("BANSTRATS_RPC_AI_DATA_SERVER")
	if addr == "" {
		t.Skip("set BANSTRATS_RPC_AI_DATA_SERVER to run against a feature server")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("connect grpc fail", zap.Error(err))
		return
	}
	defer conn.Close()
	c := biz.NewFeaFeederClient(conn)
	ctx := context.Background()
	startMS, _ := btime.ParseTimeMS("20241221")
	endMS, _ := btime.ParseTimeMS("20250102")
	task := "aifea"
	codes := []string{"BTC/USDT:USDT", "XRP/USDT:USDT"}
	res, err := c.SubFeatures(ctx, &biz.SubReq{
		Exchange: "binance",
		Market:   "linear",
		Codes:    codes,
		Start:    startMS,
		End:      endMS,
		Task:     task,
		Sample:   1,
	})
	fmt.Printf("\nlisten for %s\n", codes)
	if err != nil {
		log.Error("call SubFeatures fail", zap.Error(err))
		return
	}
	i := 0
	for i < 10 {
		i += 1
		fea, err := res.Recv()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				log.Info("done", zap.Strings("code", codes))
			} else {
				log.Error("receive feature fail", zap.Strings("code", codes), zap.Error(err))
			}
			break
		}
		infoData := fea.Mats["info"].Data
		dateStr := btime.ToDateStr(int64(infoData[0]*1000), core.DefaultDateFmt)
		fmt.Printf("\nbar: %v, close: %v", dateStr, infoData[4])
		for key, item := range fea.Mats {
			fmt.Printf("\n  %v shape: %v", key, item.Shape)
		}
		item := fea.Mats["feas"]
		row, col := int(item.Shape[1]), int(item.Shape[2])
		matData := item.Data[:len(item.Data)/len(fea.Codes)]
		arr := mat.NewDense(row, col, matData)
		column := make([]float64, row)
		j := 5
		for i := 0; i < row; i++ {
			column[i] = arr.At(i, j)
		}
		sta := sampleArr(column)
		fmt.Printf("\n  feas col %v: %v\n", j, staToStr(sta))
	}
}

type ColSta struct {
	Nan  int // nan数量
	Min  float64
	Max  float64
	Mean float64
}

func staToStr(sta *ColSta) string {
	return fmt.Sprintf("nan:%v, mean: %.5f, [%.5f, %.5f]", sta.Nan, sta.Mean, sta.Min, sta.Max)
}
func sampleArr(arr []float64) *ColSta {
	nanNum := 0
	sum := float64(0)
	valid := 0
	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	for _, v := range arr {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			nanNum += 1
			continue
		}
		sum += v
		valid += 1
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if valid > 0 {
		sum /= float64(valid)
	}
	return &ColSta{Nan: nanNum, Min: minVal, Max: maxVal, Mean: sum}
}
