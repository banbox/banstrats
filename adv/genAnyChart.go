package adv

import (
	_ "embed"
	"flag"
	"fmt"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/utils"
	"github.com/banbox/banexg"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/exg"
	"github.com/banbox/banbot/opt"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

//go:embed chartLine.html
var chartTpl []byte

func genChartDemo() {
	values := make([]float64, 100)
	labels := make([]string, 100)
	for i := range values {
		values[i] = math.Pow(float64(i), 1.5)
		labels[i] = strconv.Itoa(i)
	}
	g := opt.Chart{
		Labels: labels,
		Datasets: []*opt.ChartDs{
			{
				Label:       "pow(x, 1.5)",
				Data:        values,
				BorderColor: "rgb(75, 192, 192)",
			},
		},
	}
	g.DumpFile("D:/dump_pairs2.html")
}

// GenAnyChart 生成任意K线图表示例
func genAnyChart(args []string) error {
	var exgName, market, symbol, tf, timeRange string
	var parser = flag.NewFlagSet("", flag.ExitOnError)
	parser.StringVar(&exgName, "exchange", "binance", "exchange name")
	parser.StringVar(&market, "market", "spot", "market type: spot/linear/inverse")
	parser.StringVar(&symbol, "symbol", "BTC/USDT", "symbol")
	parser.StringVar(&tf, "timeframe", "1h", "time frame: 1m, 5m, 15, 1h, 1d ...")
	parser.StringVar(&timeRange, "timerange", "20241201-20250101", "start-end")

	err_ := parser.Parse(args)
	if err_ != nil {
		return err_
	}

	klines := getKline(exgName, market, symbol, tf, timeRange)

	// 4. 准备图表数据
	labels := make([]string, len(klines))
	prices := make([]float64, len(klines))

	for i, k := range klines {
		labels[i] = time.Unix(k.Time/1000, 0).Format("2006-01-02")
		prices[i] = k.Close
	}

	// 5. 创建数据集
	datasets := []*opt.ChartDs{
		{
			Label:       fmt.Sprintf("%s Close Price", symbol),
			Data:        prices,
			BorderColor: "rgb(75, 192, 192)",
		},
	}

	// 6. 生成图表
	title := fmt.Sprintf("%s %s %s Chart", exgName, symbol, tf)
	g := &opt.Chart{
		TplData:   chartTpl,
		Title:     title,
		Labels:    labels,
		Datasets:  datasets,
		Precision: 2,
	}

	// 7. 保存并打开文件
	workDir, err_ := os.Getwd()
	if err_ != nil {
		return err_
	}
	outDir := filepath.Join(workDir, "charts")
	if err_ = os.MkdirAll(outDir, 0755); err_ != nil {
		return err_
	}

	symbolClean := strings.ReplaceAll(strings.ReplaceAll(symbol, "/", ""), ":", "_")
	outPath := filepath.Join(outDir, fmt.Sprintf("%s_%s_%s.html", exgName, symbolClean, tf))

	if err := g.DumpFile(outPath); err != nil {
		return err
	}

	log.Info("chart generated", zap.String("path", outPath))
	_ = utils.OpenBrowser("file://" + outPath)
	return nil
}

func getKline(exgName, market, symbol, tf, timeRange string) []*banexg.Kline {
	// 1. 初始化
	core.SetRunMode(core.RunModeOther)
	if err := biz.SetupComs(&config.CmdArgs{}); err != nil {
		panic(err)
	}

	// 3. 获取K线数据
	exchange, err := exg.GetWith(exgName, market, "")
	if err != nil {
		panic(err)
	}
	err = orm.InitExg(exchange)
	if err != nil {
		panic(err)
	}
	exs, err := orm.GetExSymbol(exchange, symbol)
	if err != nil {
		panic(err)
	}

	startMS, endMS, err_ := config.ParseTimeRange(timeRange)
	if err_ != nil {
		panic(err_)
	}
	// auto download and get klines
	_, klines, err := orm.AutoFetchOHLCV(exchange, exs, tf, startMS, endMS, 0, false, nil)
	if err != nil {
		panic(err)
	}
	if len(klines) == 0 {
		panic(errs.NewMsg(errs.CodeRunTime, "no kline data got"))
	}
	return klines
}
