// Command backtest downloads checked public daily candles and runs the native
// banbot weight engine. It requires no exchange credentials or database.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/backtest"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banstrats/examples/crosssection"
	"gopkg.in/yaml.v3"
)

const day = int64(86400000)
const assumption = "权重回测的理想下一日开盘观察假设：日K收盘在下一UTC日00:00可见，真实次日open在00:00:01以独立daily-prices/1d流可见；1秒仅仿真接收延时，并非真实tick或1m，不可用于events。每天23:59:59.999另提供该日真实close估值观察以标记持仓及次日决策预算；该观察发生在新决策前，不用于撮合新决策。最后close估值不平仓。手续费和滑点各0.05%，资金费explicit-zero（忽略真实资金费），无盘口/成交量约束。固定12资产有幸存者偏差，结果为教学示例。"

var symbols = []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "XRPUSDT", "ADAUSDT", "DOGEUSDT", "SOLUSDT", "DOTUSDT", "LINKUSDT", "LTCUSDT", "BCHUSDT", "AVAXUSDT"}

type candle struct {
	At                                          int64
	Open, High, Low, Close, Volume, QuoteVolume float64
}
type sourceFile struct {
	Symbol, URL, ChecksumURL, SHA256, Month string
	Rows                                    int
}
type point struct {
	At  int64
	NAV float64
}
type summary struct {
	Strategy    string  `json:"strategy"`
	NetReturn   float64 `json:"net_return"`
	Annualized  float64 `json:"annualized_return"`
	MaxDrawdown float64 `json:"max_drawdown"`
	Sharpe      float64 `json:"daily_sharpe_365"`
	FinalNAV    float64 `json:"final_nav"`
	Turnover    float64 `json:"turnover_notional"`
	Fees        float64 `json:"fees"`
	Slippage    float64 `json:"slippage"`
	Executions  int     `json:"executions"`
	Skipped     int     `json:"skipped_warmup_or_no_target"`
}

func main() {
	out := flag.String("out", "examples/crosssection/reports/2024-2025", "report and source-cache directory")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(out string) error {
	out, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(out, "cache"), 0755); err != nil {
		return err
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store, err := factor.NewVersionStore(40000)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 45 * time.Second}
	var files []sourceFile
	sids := make([]int32, len(symbols))
	sidMap := map[int32]string{}
	for i, symbol := range symbols {
		sid := int32(i + 1)
		sids[i] = sid
		sidMap[sid] = symbol
		var candles []candle
		for month := from; month.Before(to); month = month.AddDate(0, 1, 0) {
			filename := fmt.Sprintf("%s-1d-%s.zip", symbol, month.Format("2006-01"))
			url := "https://data.binance.vision/data/futures/um/monthly/klines/" + symbol + "/1d/" + filename
			raw, hash, err := downloadChecked(client, url, filepath.Join(out, "cache", filename))
			if err != nil {
				return fmt.Errorf("%s: %w", filename, err)
			}
			rows, err := parseZip(raw, month)
			if err != nil {
				return fmt.Errorf("%s: %w", filename, err)
			}
			candles = append(candles, rows...)
			files = append(files, sourceFile{symbol, url, url + ".CHECKSUM", hash, month.Format("2006-01"), len(rows)})
		}
		if len(candles) != int(to.Sub(from).Hours()/24) {
			return fmt.Errorf("%s incomplete history", symbol)
		}
		for j, c := range candles {
			if c.At != from.UnixMilli()+int64(j)*day {
				return fmt.Errorf("%s noncontinuous daily history", symbol)
			}
			series := orm.DataSeries{Source: "kline", Sid: sid, TimeMS: c.At, EndMS: c.At + day, TimeFrame: "1d", Closed: true, Values: map[string]any{"open": c.Open, "high": c.High, "low": c.Low, "close": c.Close, "volume": c.Volume, "quote_volume": c.QuoteVolume}}
			if err = store.Put(factor.VersionRecord{Series: series, EventTime: c.At + day, AvailableAt: c.At + day, IngestedAt: c.At + day, Revision: 1, SourceVersion: "binance-vision-daily-v1"}); err != nil {
				return err
			}
			if err = putPrice(store, sid, c.At+1000, c.Open); err != nil {
				return err
			}
			if err = putPrice(store, sid, c.At+day-1, c.Close); err != nil {
				return err
			}
		}
		fmt.Printf("download verified %s: %d daily bars\n", symbol, len(candles))
	}
	archive := filepath.Join(out, "daily.gob")
	digest, err := store.Export(archive)
	if err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(out, "sources.json"), map[string]any{"assumptions": assumption, "from": from, "to_exclusive": to, "archive_content_hash": digest, "sid_map": sidMap, "files": files, "banstrats_revision": codeRevision(), "banbot_revision": gitRevision("../banbot")}); err != nil {
		return err
	}
	revision := codeRevision()
	var summaries []summary
	curves := map[string][]point{}
	for _, name := range crosssection.Names() {
		dir := filepath.Join(out, name)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		cfg := runner.Config{Definition: name, Mode: runner.Weights, StrategyID: name, AccountID: "weights-usdt", InitialNAV: 10000, MaxRecords: 40000, MaxPending: 8, DecisionInterval: day, LatencyMS: 1, ExpiryMS: day - 1, Chunks: []runner.Chunk{{Path: archive, From: from.UnixMilli() + day, To: to.UnixMilli() - 1}}, Prices: runner.PriceStream{Source: "daily-prices", TimeFrame: "1d", Field: "price"}, Factor: research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1d"}, Snapshot: factor.SnapshotSpec{Universe: factor.Universe{Version: "static-12-usdt-v1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Tracked: sids, Static: true}, SIDMap: sidMap, Schemas: map[string]string{"kline": "ohlcv-quote-volume-v1", "daily-prices": "price-v1-terminal-close"}, SourceVersions: map[string]string{"kline": "binance-vision-daily-v1", "daily-prices": "ideal-next-open-terminal-close-v1"}, AdjustmentVersion: "raw", VisibilityPolicy: "available-at"}, Manifest: research.ManifestSpec{Currency: "USDT", CodeRevision: revision, Parameters: map[string]float64{}, Portfolio: research.PortfolioDefinition{K: 3, LongNotional: .5, ShortNotional: .5, Mode: factor.Full}, Costs: research.CostSpec{FeeRate: .0005, SlippageRate: .0005, FundingPolicy: "explicit-zero"}}, ArtifactPath: filepath.Join(dir, "run.json")}
		if err = writeConfig(filepath.Join(dir, "config.yml"), cfg); err != nil {
			return err
		}
		events, err := os.Create(filepath.Join(dir, "events.jsonl"))
		if err != nil {
			return err
		}
		output := &reportOutput{JSONOutput: &runner.JSONOutput{Writer: events, SIDs: sids}}
		result, runErr := runner.Run(context.Background(), cfg, nil, output)
		closeErr := events.Close()
		if runErr != nil {
			return fmt.Errorf("%s: %w", name, runErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if result.Incomplete != 0 || result.Unresolved != 0 || result.Executions < 600 {
			return fmt.Errorf("%s invalid replay: incomplete=%d unresolved=%d executions=%d", name, result.Incomplete, result.Unresolved, result.Executions)
		}
		points := dailyPoints(output.points, from.UnixMilli(), to.UnixMilli()-1, result.Book.NAV)
		stat, err := statistics(name, points, result)
		if err != nil {
			return err
		}
		summaries = append(summaries, stat)
		curves[name] = points
		if err = writeJSON(filepath.Join(dir, "summary.json"), stat); err != nil {
			return err
		}
		if err = writeJSON(filepath.Join(dir, "manifest.json"), map[string]any{"runner_manifest": result.Manifest, "manifest_id": result.ManifestID, "assumptions": assumption, "nav_sampling": "daily open at UTC00:00:01, after rebalance; final actual close appended; terminal partial day excluded from daily Sharpe", "cash_warmup": "Jan1没有前日完成bar，首决策Jan2；各定义在滚动窗口成熟前持有现金；资金与收益统计均从Jan1开始", "terminal_valuation_at": to.UnixMilli() - 1, "terminal_positions_open": true}); err != nil {
			return err
		}
		if err = writeEquity(filepath.Join(dir, "equity.csv"), points); err != nil {
			return err
		}
		fmt.Printf("%s: return %.2f%%, executions %d\n", name, stat.NetReturn*100, stat.Executions)
	}
	if err = writeJSON(filepath.Join(out, "summary.json"), summaries); err != nil {
		return err
	}
	if err = writeSummaryCSV(filepath.Join(out, "summary.csv"), summaries); err != nil {
		return err
	}
	return writeReport(out, summaries, curves)
}
func putPrice(store *factor.VersionStore, sid int32, at int64, price float64) error {
	return store.Put(factor.VersionRecord{Series: orm.DataSeries{Source: "daily-prices", Sid: sid, TimeMS: at, EndMS: at, TimeFrame: "1d", Values: map[string]any{"price": price}}, EventTime: at, AvailableAt: at, IngestedAt: at, Revision: 1, SourceVersion: "ideal-next-open-terminal-close-v1"})
}
func fetch(client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d for %s", resp.StatusCode, url)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("HTTP payload exceeds limit")
	}
	return raw, nil
}
func downloadChecked(client *http.Client, url, path string) ([]byte, string, error) {
	checkPath := path + ".CHECKSUM"
	checksum, err := os.ReadFile(checkPath)
	if errors.Is(err, os.ErrNotExist) {
		checksum, err = fetch(client, url+".CHECKSUM", 4096)
		if err == nil {
			err = os.WriteFile(checkPath, checksum, 0644)
		}
	}
	if err != nil {
		return nil, "", err
	}
	fields := strings.Fields(string(checksum))
	if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != filepath.Base(path) {
		return nil, "", errors.New("malformed source checksum")
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil || len(expected) != sha256.Size {
		return nil, "", errors.New("invalid SHA256 checksum")
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = fetch(client, url, 8<<20)
	}
	if err != nil {
		return nil, "", err
	}
	actual := sha256.Sum256(raw)
	if !bytes.Equal(expected, actual[:]) {
		return nil, "", errors.New("source SHA256 mismatch (remove corrupt cache and retry)")
	}
	if err = os.WriteFile(path, raw, 0644); err != nil {
		return nil, "", err
	}
	return raw, hex.EncodeToString(actual[:]), nil
}
func parseZip(raw []byte, month time.Time) ([]candle, error) {
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	if len(z.File) != 1 || !strings.HasSuffix(z.File[0].Name, ".csv") || z.File[0].UncompressedSize64 > 8<<20 {
		return nil, errors.New("unexpected daily archive")
	}
	reader, err := z.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return parseCSV(io.LimitReader(reader, 8<<20), month)
}
func parseCSV(input io.Reader, month time.Time) ([]candle, error) {
	r := csv.NewReader(input)
	r.FieldsPerRecord = -1
	var result []candle
	end := month.AddDate(0, 1, 0).UnixMilli()
	for line := 1; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if line == 1 && len(row) > 0 && row[0] == "open_time" {
			continue
		}
		if len(row) != 12 {
			return nil, fmt.Errorf("CSV line %d: expected 12 fields", line)
		}
		at, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			return nil, err
		}
		if at != month.UnixMilli()+int64(len(result))*day || at >= end {
			return nil, errors.New("noncontinuous or invalid daily timestamp")
		}
		closeAt, err := strconv.ParseInt(row[6], 10, 64)
		if err != nil || closeAt != at+day-1 {
			return nil, errors.New("invalid daily close timestamp")
		}
		vals := make([]float64, 6)
		for j, index := range []int{1, 2, 3, 4, 5, 7} {
			v, err := strconv.ParseFloat(row[index], 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || (j < 4 && v == 0) {
				return nil, errors.New("invalid finite price/volume")
			}
			vals[j] = v
		}
		c := candle{at, vals[0], vals[1], vals[2], vals[3], vals[4], vals[5]}
		if c.Low > math.Min(c.Open, c.Close) || c.High < math.Max(c.Open, c.Close) || c.High < c.Low {
			return nil, errors.New("inconsistent OHLC")
		}
		result = append(result, c)
	}
	if len(result) != int((end-month.UnixMilli())/day) {
		return nil, errors.New("incomplete monthly daily CSV")
	}
	return result, nil
}

type reportOutput struct {
	*runner.JSONOutput
	points []point
}

func (o *reportOutput) Executed(p *factor.TargetPortfolio, s backtest.State, at int64) error {
	o.points = append(o.points, point{at, s.NAV})
	return o.JSONOutput.Executed(p, s, at)
}
func (o *reportOutput) TargetAccepted(p *factor.TargetPortfolio, s backtest.State, at int64) error {
	return o.Executed(p, s, at)
}

// Sample after each day's open rebalance; append the real final close mark.
func dailyPoints(executions []point, from, to int64, final float64) []point {
	points := []point{{from, 10000}}
	index := 0
	nav := 10000.0
	for at := from + day; at < to; at += day {
		for index < len(executions) && executions[index].At <= at+1000 {
			nav = executions[index].NAV
			index++
		}
		points = append(points, point{at + 1000, nav})
	}
	points = append(points, point{to, final})
	return points
}
func statistics(name string, points []point, r runner.Result) (summary, error) {
	s := summary{Strategy: name, FinalNAV: r.Book.NAV, Turnover: r.Book.Turnover, Fees: r.Book.Fees, Slippage: r.Book.Slippage, Executions: r.Executions, Skipped: r.Skipped}
	if len(points) < 2 {
		return s, errors.New("insufficient equity observations")
	}
	peak := points[0].NAV
	returns := make([]float64, 0, len(points)-1)
	for i, p := range points {
		if p.NAV <= 0 || math.IsNaN(p.NAV) || math.IsInf(p.NAV, 0) {
			return s, errors.New("nonpositive/nonfinite NAV")
		}
		peak = math.Max(peak, p.NAV)
		s.MaxDrawdown = math.Max(s.MaxDrawdown, 1-p.NAV/peak)
		if i > 0 {
			if p.At <= points[i-1].At {
				return s, errors.New("unordered equity")
			}
			// The terminal close is less than one day after the last open.
			// Include it in total return/drawdown, not daily Sharpe.
			if p.At-points[i-1].At >= day {
				returns = append(returns, p.NAV/points[i-1].NAV-1)
			}
		}
	}
	s.NetReturn = points[len(points)-1].NAV/points[0].NAV - 1
	years := float64(points[len(points)-1].At-points[0].At) / float64(day) / 365
	s.Annualized = math.Pow(1+s.NetReturn, 1/years) - 1
	mean := 0.0
	for _, v := range returns {
		mean += v
	}
	if len(returns) == 0 {
		return s, nil
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, v := range returns {
		variance += (v - mean) * (v - mean)
	}
	if len(returns) > 1 {
		variance /= float64(len(returns) - 1)
	}
	if variance > 0 {
		s.Sharpe = mean / math.Sqrt(variance) * math.Sqrt(365)
	}
	return s, nil
}
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
}
func writeConfig(path string, c runner.Config) error {
	// Export the example's replay settings as ordinary shallow strategy YAML.
	// runner.Config is an internal API, not a supported CLI configuration format.
	name := c.Definition
	if name == "" {
		name = c.StrategyID
	}
	policy := map[string]any{
		"name": name, "id": c.StrategyID, "account": c.AccountID, "engine": "factor",
		"run_timeframes": []string{c.Factor.TimeFrame}, "params": c.Manifest.Parameters,
		"chunks": c.Chunks, "prices": c.Prices, "initial_nav": c.InitialNAV, "max_records": c.MaxRecords,
		"decision":  map[string]any{"interval_ms": c.DecisionInterval, "delay_ms": c.DecisionDelayMS, "latency_ms": c.LatencyMS, "expiry_ms": c.ExpiryMS, "max_pending": c.MaxPending},
		"portfolio": map[string]any{"k": c.Manifest.Portfolio.K, "long_notional": c.Manifest.Portfolio.LongNotional, "short_notional": c.Manifest.Portfolio.ShortNotional, "mode": c.Manifest.Portfolio.Mode},
		"research":  map[string]any{"labels": []any{}},
		"manifest": map[string]any{
			"currency": c.Manifest.Currency, "code_revision": c.Manifest.CodeRevision,
			"costs": map[string]any{"fee_rate": c.Manifest.Costs.FeeRate, "slippage_rate": c.Manifest.Costs.SlippageRate, "funding_policy": c.Manifest.Costs.FundingPolicy},
		},
		"snapshot": map[string]any{
			"universe": c.Snapshot.Universe, "sid_map": c.Snapshot.SIDMap, "schemas": c.Snapshot.Schemas,
			"source_versions": c.Snapshot.SourceVersions, "adjustment_version": c.Snapshot.AdjustmentVersion, "visibility_policy": c.Snapshot.VisibilityPolicy,
		},
	}
	if c.Expressions != nil {
		policy["expressions"] = c.Expressions
	}
	raw, err := yaml.Marshal(map[string]any{
		"accounts":   map[string]any{c.AccountID: map[string]any{}},
		"execution":  map[string]any{"mode": c.Mode, "funding_policy": c.Manifest.Costs.FundingPolicy},
		"run_policy": []any{policy},
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
func writeEquity(path string, points []point) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"utc", "nav"})
	for _, p := range points {
		_ = w.Write([]string{time.UnixMilli(p.At).UTC().Format(time.RFC3339Nano), strconv.FormatFloat(p.NAV, 'f', 8, 64)})
	}
	w.Flush()
	return errors.Join(w.Error(), f.Close())
}
func writeSummaryCSV(path string, rows []summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"strategy", "net_return", "annualized_return", "max_drawdown", "daily_sharpe_365", "final_nav", "turnover_notional", "fees", "slippage", "executions", "skipped"})
	for _, s := range rows {
		_ = w.Write([]string{s.Strategy, fmt.Sprint(s.NetReturn), fmt.Sprint(s.Annualized), fmt.Sprint(s.MaxDrawdown), fmt.Sprint(s.Sharpe), fmt.Sprint(s.FinalNAV), fmt.Sprint(s.Turnover), fmt.Sprint(s.Fees), fmt.Sprint(s.Slippage), strconv.Itoa(s.Executions), strconv.Itoa(s.Skipped)})
	}
	w.Flush()
	return errors.Join(w.Error(), f.Close())
}
func codeRevision() string {
	sum := sha256.New()
	for _, name := range []string{"examples/crosssection/strategies.go", "examples/crosssection/backtest/main.go"} {
		b, err := os.ReadFile(name)
		if err == nil {
			_, _ = sum.Write(b)
		}
	}
	return gitRevision(".") + "+examples-sha256:" + hex.EncodeToString(sum.Sum(nil))
}
func gitRevision(dir string) string {
	raw, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	revision := strings.TrimSpace(string(raw))
	dirty, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err == nil && len(dirty) > 0 {
		revision += "+dirty"
	}
	return revision
}
func writeReport(out string, rows []summary, curves map[string][]point) error {
	var md strings.Builder
	md.WriteString("资产池：" + strings.Join(symbols, ", ") + "。\n\n")
	md.WriteString("# 经典截面策略回测报告\n\n区间：2024-01-01 至 2025-12-31 UTC；每策略独立初始资金 10,000 USDT。公开 Binance USDT 永续日线，12资产，24个月，SHA256逐文件验证。\n\n" + assumption + "\n\n每日收盘决策：分数最高3个做多、最低3个做空，多空各50%净值，每日再平衡。首日没有前日完成bar，首个决策Jan2；各定义窗口成熟前持有现金，此空置时间包含在收益/年化/Sharpe统计中。终端按最后真实close估值，持仓未平仓、不扣虚拟平仓费用。\n\n|策略|净收益|年化|最大回撤|日Sharpe|执行次数|手续费|滑点|周转金额|\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, s := range rows {
		fmt.Fprintf(&md, "|%s|%.2f%%|%.2f%%|%.2f%%|%.3f|%d|%.2f|%.2f|%.2f|\n", s.Strategy, s.NetReturn*100, s.Annualized*100, s.MaxDrawdown*100, s.Sharpe, s.Executions, s.Fees, s.Slippage, s.Turnover)
	}
	md.WriteString("\n收益已扣手续费/滑点；年化按实际日数/365，Sharpe使用每日00:00:01开盘/再平衡后净收益的样本标准差、365天、无风险利率0；前期现金日包含，终端close不足1日收益不计入日Sharpe。最大回撤为每日采样与末日close估值的最大回撤，不代表日内最大回撤。前期Skipped为滚动窗口未成熟或有效截面不足，不表示已成交；所有运行检查Incomplete=0、Unresolved=0、Executions>=600。策略分数详情和原生事件见各目录events.jsonl；run.json为runner原生结果，manifest.json说明假设，equity.csv为净值采样。\n\n重跑下载与报告：从banstrats根目录执行 `go run ./examples/crosssection/backtest`。缓存保留在cache，重复执行校验哈希。重跑单策略：`go run . backtest --mode weights --no-default --datadir examples/crosssection/reports/2024-2025/replay --config examples/crosssection/reports/2024-2025/CSMomentum/config.yml`（先确保主入口已导入crosssection注册包）。config.yml采用普通浅层YAML策略配置；archive路径为本机绝对路径，迁移后需更新。\n")
	if err := os.WriteFile(filepath.Join(out, "REPORT.md"), []byte(md.String()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "README.md"), []byte("请查看 [REPORT.md](REPORT.md) 与 [report.html](report.html)。数据来源、checksum URL、哈希及SID映射见sources.json。\n"), 0644); err != nil {
		return err
	}
	minNAV, maxNAV := 10000.0, 10000.0
	for _, ps := range curves {
		for _, p := range ps {
			minNAV = math.Min(minNAV, p.NAV)
			maxNAV = math.Max(maxNAV, p.NAV)
		}
	}
	if maxNAV == minNAV {
		maxNAV++
	}
	colors := []string{"#2563eb", "#dc2626", "#059669", "#7c3aed", "#d97706", "#0891b2", "#db2777"}
	var svg strings.Builder
	svg.WriteString(`<svg viewBox="0 0 1000 500" role="img" aria-label="各策略净值曲线"><rect width="1000" height="500" fill="#fafafa"/>`)
	for i, s := range rows {
		ps := curves[s.Strategy]
		fmt.Fprintf(&svg, `<polyline fill="none" stroke="%s" stroke-width="1.5" points="`, colors[i%len(colors)])
		for j, p := range ps {
			fmt.Fprintf(&svg, "%.2f,%.2f ", 40+920*float64(j)/float64(len(ps)-1), 460-400*(p.NAV-minNAV)/(maxNAV-minNAV))
		}
		svg.WriteString(`"/>`)
		fmt.Fprintf(&svg, `<text x="%d" y="20" font-size="11" fill="%s">%s</text>`, i*140+10, colors[i%len(colors)], html.EscapeString(s.Strategy))
	}
	fmt.Fprintf(&svg, `<text x="10" y="60">%.0f</text><text x="10" y="480">%.0f</text></svg>`, maxNAV, minNAV)
	body := `<!doctype html><html lang="zh"><meta charset="utf-8"><title>经典截面策略回测报告</title><style>body{font-family:system-ui;max-width:1200px;margin:32px auto;padding:0 20px}svg{width:100%}pre{white-space:pre-wrap;line-height:1.6}</style><h1>2024–2025 截面策略净值</h1>` + svg.String() + "<pre>" + html.EscapeString(md.String()) + "</pre></html>"
	return os.WriteFile(filepath.Join(out, "report.html"), []byte(body), 0644)
}
