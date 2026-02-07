package adv

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/utils"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	utils2 "github.com/banbox/banexg/utils"
	ta "github.com/banbox/banta"
	"go.uber.org/zap"
)

//go:embed klinechart.html
var klineTpl []byte

func genKlineChart(args *config.CmdArgs) *errs.Error {
	exgName, market, timeRange := "binance", "spot", "20241001-20250101"
	symbol := args.RawPairs
	if symbol == "" {
		symbol = "BTC/USDT"
	}
	tf := args.RawTimeFrames
	if tf == "" {
		tf = "1d"
	}
	if args.TimeRange != "" {
		timeRange = args.TimeRange
	}
	warmUpNum := 20
	klines := getKline(exgName, market, symbol, tf, timeRange)

	// 4. 准备图表数据
	klineData := make([]map[string]interface{}, len(klines))
	kdjData := make([]map[string]float64, len(klines))

	// 创建TA计算环境
	e := &ta.BarEnv{
		TimeFrame:  tf,
		TFMSecs:    int64(utils2.TFToSecs(tf) * 1000),
		Exchange:   exgName,
		MarketType: market,
	}
	e.Reset()

	for i, k := range klines {
		// calculate indicators
		e.OnBar(k.Time, k.Open, k.High, k.Low, k.Close, k.Volume, k.Quote, k.BuyVolume, k.TradeNum)
		kdjK, kdjD, _ := ta.KDJ(e.High, e.Low, e.Close, 9, 3, 3)

		if i < warmUpNum {
			continue
		}

		klineData[i] = map[string]interface{}{
			"timestamp": k.Time,
			"open":      k.Open,
			"high":      k.High,
			"low":       k.Low,
			"close":     k.Close,
			"volume":    k.Volume,
		}
		kdjData[i] = map[string]float64{
			"k": kdjK.Get(0),
			"d": kdjD.Get(0),
		}
	}

	// 5. 准备模板数据
	templateData := map[string]interface{}{
		"Title":     fmt.Sprintf("%s %s %s", exgName, symbol, tf),
		"KLineData": klineData,
		"KDJData":   kdjData,
	}

	// 6. 读取并解析模板
	tmpl := template.Must(template.New("kline").Parse(string(klineTpl)))

	// 7. 保存文件
	workDir, err := os.Getwd()
	if err != nil {
		return errs.New(errs.CodeIOReadFail, err)
	}
	outDir := filepath.Join(workDir, "charts")
	if err = os.MkdirAll(outDir, 0755); err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}

	symbolClean := strings.ReplaceAll(strings.ReplaceAll(symbol, "/", ""), ":", "_")
	outPath := filepath.Join(outDir, fmt.Sprintf("%s_%s_%s_kdj.html", exgName, symbolClean, tf))

	file, err := os.Create(outPath)
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	defer file.Close()

	if err = tmpl.Execute(file, templateData); err != nil {
		return errs.New(errs.CodeRunTime, err)
	}

	log.Info("chart generated", zap.String("path", outPath))
	_ = utils.OpenBrowser("file://" + outPath)
	return nil
}
