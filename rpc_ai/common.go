package rpc_ai

import (
	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banbot/utils"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	ta "github.com/banbox/banta"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"math"
	"sync"
)

func genFutProbs(arr []*banexg.Kline, minChgVal float64, maxRevRate float64) [3]float64 {
	first, upper, lower, chgVal := getFutBounds(arr)
	total := upper - lower
	chgRate := chgVal / first
	upRate := (upper - first) / total
	dnRate := (first - lower) / total
	revRate := min(upRate, dnRate) / max(upRate, dnRate)
	var zeroRate float64
	if revRate > maxRevRate {
		// 反向波动过大，视为不确定
		zeroRate = min(1.0, max(0.55, revRate+0.2))
	} else if chgRate < 0.005 || chgVal < minChgVal {
		// 波动太小，视为不确定
		var rateScore, valScore float64
		if chgRate < 0.01 {
			rateScore = 1 - chgRate/0.01
		}
		if chgVal < minChgVal*2 {
			valScore = 1 - chgVal/minChgVal/2
		}
		zeroRate = max(0.55, rateScore*0.5+valScore*0.5)
	} else {
		// 有方向的行情，将反向波动50%作为不确定的概率
		zeroRate = revRate * 0.5
	}
	dirtRate := 1 - zeroRate
	upRate *= dirtRate
	dnRate *= dirtRate
	return [3]float64{zeroRate, upRate, dnRate}
}

func getFutBounds(arr []*banexg.Kline) (float64, float64, float64, float64) {
	var first = arr[0].Open
	var lower, upper = first, first
	var lowAt, uppAt = 0, 0
	for i, b := range arr {
		if b.Low < lower {
			lowAt = i
			lower = b.Low
		}
		if b.High > upper {
			uppAt = i
			upper = b.High
		}
	}
	var chgVal float64
	if upper-first > first-lower {
		// 属于上升趋势
		arr = arr[:uppAt+1]
		lower = first
		for _, b := range arr {
			lower = min(lower, b.Low)
		}
		chgVal = upper - first
	} else {
		// 下降趋势
		arr = arr[:lowAt+1]
		upper = first
		for _, b := range arr {
			upper = max(upper, b.High)
		}
		chgVal = first - lower
	}
	return first, upper, lower, chgVal
}

func matToNumArr(m *mat.Dense) *biz.NumArr {
	data := m.RawMatrix().Data
	row, col := m.Dims()
	return &biz.NumArr{Data: data, Shape: []int32{int32(row), int32(col)}}
}

func onAiFeatures(e *ta.BarEnv, num int) *mat.Dense {
	if e == nil {
		return nil
	}
	return onAiSeries(e.High, e.Low, e.Close, e.Volume, num)
}

func onAiDataFeatures(data *strat.DataFields, num int) *mat.Dense {
	if data == nil {
		return nil
	}
	return onAiSeries(data.Series("high"), data.Series("low"), data.Series("close"), data.Series("volume"), num)
}

func onAiSeries(high, low, close, volume *ta.Series, num int) *mat.Dense {
	if high == nil || low == nil || close == nil || volume == nil || num == 0 {
		return nil
	}
	rsi14 := ta.RSI(close, 14)
	rsi50 := ta.RSI(close, 50)
	atr14 := ta.ATR(high, low, close, 14)
	atr50 := ta.ATR(high, low, close, 30)
	sma5 := ta.SMA(close, 5)
	sma20 := ta.SMA(close, 20)
	sma70 := ta.SMA(close, 70)
	adx := ta.ADX(high, low, close, 14)
	roc := ta.ROC(close, 30)
	// 这里的特征列数最好是4或10的倍数，否则模型端采样会有问题
	feaNum := 30
	res := mat.NewDense(num, feaNum, nil)
	for i := 0; i < num; i++ {
		closeVal := close.Get(i)
		volVal := volume.Get(i)
		sma5Val := sma5.Get(i)
		sma20Val := sma20.Get(i)
		row := []float64{
			high.Get(i),
			low.Get(i),
			closeVal,
			volVal,
			(closeVal - close.Get(i+1)) / closeVal,
			(closeVal - close.Get(i+7)) / closeVal, // 此行超出(-1,1)，调用方需截断
			rsi14.Get(i) / 100,
			rsi50.Get(i) / 100,
			atr14.Get(i) / closeVal,
			atr50.Get(i) / closeVal,
			sma5Val/closeVal - 1,
			sma20Val/sma5Val - 1,
			sma70.Get(i)/sma20Val - 1,
			adx.Get(i) / 100,
			roc.Get(i) / 100, // 此行超出(-1,1)，调用方需截断
		}
		for j, v := range row {
			if math.IsNaN(v) {
				v = 0
			}
			res.Set(i, j, v)
		}
	}
	// 对前四列hlcv进行归一化
	for i := 0; i < 4; i++ {
		col := res.ColView(i)
		minVal := mat.Min(col)
		maxVal := mat.Max(col)
		if minVal == maxVal {
			continue
		}
		for row := 0; row < num; row++ {
			oldValue := res.At(row, i)
			newValue := (oldValue - minVal) / (maxVal - minVal)
			res.Set(row, i, newValue)
		}
	}
	return res
}

type BatchMap struct {
	codes    []string
	envs     []*ta.BarEnv
	mux      sync.Mutex
	curBarMS int64 // 当前bar的13位毫秒时间戳
	data     map[string]*biz.NumArr
	maxSize  int
}

func NewBatchMap(size int) *BatchMap {
	return &BatchMap{
		data:    make(map[string]*biz.NumArr),
		maxSize: size,
	}
}

func (b *BatchMap) Add(rsp biz.FeaFeeder_SubFeaturesServer, e *ta.BarEnv, barMs int64, items map[string]*biz.NumArr) *errs.Error {
	b.mux.Lock()
	defer b.mux.Unlock()
	if barMs > b.curBarMS {
		oldLen := len(b.codes)
		if oldLen > 0 {
			b.maxSize = max(b.maxSize, oldLen)
			corrs, err_ := getEnvsCorr(b.envs, 100)
			if err_ != nil {
				return errs.New(errs.CodeRunTime, err_)
			}
			b.data["corr"] = &biz.NumArr{
				Data:  corrs,
				Shape: []int32{int32(oldLen), 2},
			}
			err_ = rsp.Send(&biz.ArrMap{
				Codes: b.codes,
				Mats:  b.data,
			})
			if err_ != nil {
				return errs.New(errs.CodeRunTime, err_)
			}
			b.codes = make([]string, 0, b.maxSize)
			b.data = make(map[string]*biz.NumArr)
			b.envs = make([]*ta.BarEnv, 0, b.maxSize)
		}
		b.curBarMS = barMs
	}
	b.codes = append(b.codes, e.Symbol)
	b.envs = append(b.envs, e)
	for name, arr := range items {
		old, ok := b.data[name]
		if !ok {
			arr.Shape = append([]int32{1}, arr.Shape...)
			b.data[name] = arr
		} else {
			old.Data = append(old.Data, arr.Data...)
			old.Shape = append([]int32{old.Shape[0] + 1}, old.Shape[1:]...)
		}
	}
	return nil
}

func getEnvsCorr(envs []*ta.BarEnv, hisNum int) ([]float64, error) {
	if len(envs) < 2 {
		res := make([]float64, 0, len(envs)*2)
		for range envs {
			res = append(res, 0, 0)
		}
		return res, nil
	}
	_, avgs, err := utils.CalcEnvsCorr(envs, hisNum)
	res := make([]float64, 0, len(avgs)*2)
	avg := floats.Sum(avgs) / float64(len(avgs))
	for _, v := range avgs {
		res = append(res, avg, v)
	}
	return res, err
}
