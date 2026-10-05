package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/entry"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/backtest"
	"github.com/banbox/banbot/factor/expr"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/orm"
	"gopkg.in/yaml.v3"
)

func fixtureMonth(month time.Time) string {
	var text strings.Builder
	text.WriteString("open_time,open,high,low,close,volume,close_time,quote_volume,count,taker_base,taker_quote,ignore\n")
	for at := month.UnixMilli(); at < month.AddDate(0, 1, 0).UnixMilli(); at += day {
		fmt.Fprintf(&text, "%d,10,12,9,11,0,%d,0,1,0,0,0\n", at, at+day-1)
	}
	return text.String()
}

type budgetOutput struct {
	*runner.JSONOutput
	prior   backtest.State
	start   int64
	checked int
	t       *testing.T
}

func fixturePrice(sid int32, index int) float64 {
	return 100 + float64(sid)*10 + float64(index)*float64(sid)*.1
}
func (o *budgetOutput) TargetAccepted(p *factor.TargetPortfolio, state backtest.State, at int64) error {
	if o.checked > 0 {
		priorDay := int((p.Spec().DecisionTime-o.start)/day) - 1
		expected := o.prior.Cash
		for sid, qty := range o.prior.Quantities {
			expected += qty * fixturePrice(sid, priorDay) * (1 + .001*float64(sid))
		}
		if math.Abs(p.Spec().Budget.NAV-expected) > 1e-7 {
			o.t.Fatalf("decision budget did not mark previous real close: %.10f want %.10f", p.Spec().Budget.NAV, expected)
		}
	}
	if at != p.Spec().DecisionTime+1000 {
		o.t.Fatalf("execution did not use next observable daily open: %d %+v", at, p.Spec())
	}
	o.prior = state
	o.checked++
	return o.JSONOutput.TargetAccepted(p, state, at)
}
func TestNativeRunnerUsesCloseBudgetThenNextOpen(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	store, err := factor.NewVersionStore(3000)
	if err != nil {
		t.Fatal(err)
	}
	sids := []int32{}
	sidMap := map[int32]string{}
	for sid := int32(1); sid <= 12; sid++ {
		sids = append(sids, sid)
		sidMap[sid] = fmt.Sprintf("fixture-%d", sid)
		for i := 0; i < 50; i++ {
			at := start + int64(i)*day
			open := fixturePrice(sid, i)
			close := open * (1 + .001*float64(sid))
			row := factor.VersionRecord{Series: orm.DataSeries{Source: "kline", Sid: sid, TimeMS: at, EndMS: at + day, TimeFrame: "1d", Closed: true, Values: map[string]any{"close": close, "open": open, "high": close, "low": open, "volume": 100.0}}, EventTime: at + day, AvailableAt: at + day, IngestedAt: at + day, Revision: 1, SourceVersion: "v1"}
			if err = store.Put(row); err != nil {
				t.Fatal(err)
			}
			if err = putPrice(store, sid, at+1000, open); err != nil {
				t.Fatal(err)
			}
			if err = putPrice(store, sid, at+day-1, close); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "fixture.gob")
	if _, err = store.Export(path); err != nil {
		t.Fatal(err)
	}
	cfg := runner.Config{Definition: "CSMomentum", Mode: runner.Weights, StrategyID: "test", AccountID: "test", InitialNAV: 10000, MaxRecords: 3000, MaxPending: 8, DecisionInterval: day, LatencyMS: 1, ExpiryMS: day - 1, Chunks: []runner.Chunk{{Path: path, From: start + day, To: start + 50*day - 1}}, Prices: runner.PriceStream{Source: "daily-prices", TimeFrame: "1d", Field: "price"}, Factor: research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1d"}, Snapshot: factor.SnapshotSpec{Universe: factor.Universe{Version: "test-v1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Tracked: sids, Static: true}, SIDMap: sidMap, Schemas: map[string]string{"kline": "v1", "daily-prices": "v1"}, SourceVersions: map[string]string{"kline": "v1", "daily-prices": "ideal-next-open-terminal-close-v1"}, AdjustmentVersion: "raw", VisibilityPolicy: "available-at"}, Manifest: research.ManifestSpec{Currency: "USDT", CodeRevision: "test", Portfolio: research.PortfolioDefinition{K: 3, LongNotional: .5, ShortNotional: .5, Mode: factor.Full}, Costs: research.CostSpec{FeeRate: .0005, SlippageRate: .0005, FundingPolicy: "explicit-zero"}}}
	raw, err := os.ReadFile("../expressions.yml")
	if err != nil {
		t.Fatal(err)
	}
	var overlay struct {
		Policies []struct {
			Expressions expr.Spec `yaml:"expressions"`
		} `yaml:"run_policy"`
	}
	if err := yaml.Unmarshal(raw, &overlay); err != nil || len(overlay.Policies) != 1 {
		t.Fatalf("invalid expression fixture: %v", err)
	}
	for _, name := range []string{"CSMomentum", "CSMomentumExpr", "YAML"} {
		t.Run(name, func(t *testing.T) {
			c := cfg
			c.Definition = name
			if name == "YAML" {
				c.Definition = ""
				c.Expressions = &overlay.Policies[0].Expressions
			}
			var events bytes.Buffer
			out := &budgetOutput{JSONOutput: &runner.JSONOutput{Writer: &events, SIDs: sids}, start: start, t: t}
			result, err := runner.Run(context.Background(), c, nil, out)
			if err != nil {
				t.Fatal(err)
			}
			if result.Executions < 20 || result.Incomplete != 0 || result.Unresolved != 0 || out.checked != result.Executions {
				t.Fatalf("native replay failed: %+v", result)
			}
			terminal := out.prior.Cash
			for sid, qty := range out.prior.Quantities {
				terminal += qty * fixturePrice(sid, 49) * (1 + .001*float64(sid))
			}
			if math.Abs(terminal-result.Book.NAV) > 1e-7 {
				t.Fatalf("terminal real close not used: %.10f want %.10f", result.Book.NAV, terminal)
			}
			// Exercise the exported YAML through the same root command as users.
			dataDir := t.TempDir()
			configPath := filepath.Join(dataDir, "strategy.yml")
			if err := writeConfig(configPath, c); err != nil {
				t.Fatal(err)
			}
			command := entry.NewRootCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"backtest", "--mode", "weights", "--no-default", "--datadir", dataDir, "--config", configPath})
			if err := command.Execute(); err != nil {
				t.Fatalf("generated YAML replay failed: %v\n%s", err, output.String())
			}
			artifacts, err := filepath.Glob(filepath.Join(dataDir, "backtest", "*", "run.json"))
			if err != nil || len(artifacts) != 1 {
				t.Fatalf("missing replay result: %v %v", artifacts, err)
			}
			raw, err := os.ReadFile(artifacts[0])
			if err != nil {
				t.Fatal(err)
			}
			var artifact struct{ Results []runner.Result }
			if err := json.Unmarshal(raw, &artifact); err != nil {
				t.Fatal(err)
			}
			if len(artifact.Results) != 1 || artifact.Results[0].Executions != result.Executions || math.Abs(artifact.Results[0].Book.NAV-result.Book.NAV) > 1e-7 {
				t.Fatal("exported YAML changed execution count or final NAV")
			}
		})
	}
}
func TestParseCompleteMonthAndRejectBadData(t *testing.T) {
	month := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	valid := fixtureMonth(month)
	rows, err := parseCSV(strings.NewReader(valid), month)
	if err != nil || len(rows) != 29 || rows[0].Volume != 0 {
		t.Fatalf("valid leap month: %v %v", len(rows), err)
	}
	tests := map[string]string{
		"missing day":     strings.Join(strings.Split(valid, "\n")[:29], "\n"),
		"nonfinite":       strings.Replace(valid, ",10,12,", ",NaN,12,", 1),
		"zero price":      strings.Replace(valid, ",10,12,", ",0,12,", 1),
		"negative volume": strings.Replace(valid, ",11,0,", ",11,-1,", 1),
		"bad OHLC":        strings.Replace(valid, ",10,12,9,11,", ",10,9,9,11,", 1),
		"bad timestamp":   strings.Replace(valid, fmt.Sprint(month.UnixMilli()), fmt.Sprint(month.UnixMilli()+1), 1),
		"bad close":       strings.Replace(valid, fmt.Sprint(month.UnixMilli()+day-1), fmt.Sprint(month.UnixMilli()+day), 1),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCSV(strings.NewReader(input), month); err == nil {
				t.Fatal("invalid market input accepted")
			}
		})
	}
}
func TestDownloadChecksumCacheAndCorruption(t *testing.T) {
	payload := []byte("official-like archive fixture")
	hash := sha256.Sum256(payload)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/sample.zip":
			_, _ = w.Write(payload)
		case "/sample.zip.CHECKSUM":
			fmt.Fprintf(w, "%x  sample.zip\n", hash)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "sample.zip")
	for i := 0; i < 2; i++ {
		raw, digest, err := downloadChecked(server.Client(), server.URL+"/sample.zip", path)
		if err != nil || string(raw) != string(payload) || digest != fmt.Sprintf("%x", hash) {
			t.Fatalf("download: %s %s %v", raw, digest, err)
		}
	}
	if requests != 2 {
		t.Fatalf("cache should avoid network: %d requests", requests)
	}
	if err := os.WriteFile(path, []byte("corrupted"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := downloadChecked(server.Client(), server.URL+"/sample.zip", path); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	if _, err := fetch(server.Client(), server.URL+"/missing", 100); err == nil {
		t.Fatal("HTTP 404 accepted")
	}
	if _, err := fetch(server.Client(), server.URL+"/sample.zip", 1); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
func TestDailySamplingAndTerminalSharpe(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := from + 3*day - 1
	points := dailyPoints([]point{{from + day + 1000, 11000}, {from + 2*day + 1000, 9900}}, from, end, 19800)
	if len(points) != 4 || points[1].At != from+day+1000 || points[2].NAV != 9900 || points[3].At != end {
		t.Fatalf("real observation timestamps lost: %#v", points)
	}
	s, err := statistics("test", points, runner.Result{Book: backtest.State{NAV: 19800}})
	if err != nil {
		t.Fatal(err)
	}
	// Full daily returns are +10%, -10%; terminal +100% must not enter Sharpe.
	if math.Abs(s.Sharpe) > 1e-12 || math.Abs(s.NetReturn-.98) > 1e-12 || math.Abs(s.MaxDrawdown-.1) > 1e-12 {
		t.Fatalf("wrong statistics: %#v", s)
	}
	flat := []point{{from, 10000}, {from + day, 10000}}
	s, err = statistics("flat", flat, runner.Result{})
	if err != nil || s.Sharpe != 0 || s.Annualized != 0 {
		t.Fatalf("constant NAV: %#v %v", s, err)
	}
	for _, bad := range [][]point{{{from, 10000}}, {{from, 10000}, {from + day, 0}}, {{from, 10000}, {from + day, math.NaN()}}, {{from, 10000}, {from, 10000}}} {
		if _, err := statistics("bad", bad, runner.Result{}); err == nil {
			t.Fatal("invalid equity accepted")
		}
	}
}
func TestGeneratedConfigUsesCanonicalYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	c := runner.Config{
		Definition: "CSMomentum", StrategyID: "CSMomentum", AccountID: "weights-usdt", Mode: runner.Weights,
		DecisionInterval: day, LatencyMS: 1, ExpiryMS: day - 1, MaxPending: 8, MaxRecords: 1000, InitialNAV: 10000,
		Factor:   research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1d"},
		Prices:   runner.PriceStream{Source: "daily-prices", TimeFrame: "1d", Field: "price"},
		Snapshot: factor.SnapshotSpec{Universe: factor.Universe{Version: "test", Static: true}, AdjustmentVersion: "raw", VisibilityPolicy: "available-at"},
		Manifest: research.ManifestSpec{Currency: "USDT", CodeRevision: "test", Costs: research.CostSpec{FundingPolicy: "explicit-zero"}, Portfolio: research.PortfolioDefinition{K: 3, LongNotional: .5, ShortNotional: .5, Mode: factor.Full}},
	}
	if err := writeConfig(path, c); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, parseErr := config.ParseUnifiedYAML(raw, path); parseErr != nil {
		t.Fatalf("generated config rejected: %v", parseErr)
	}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) || bytes.Contains(raw, []byte("config_version")) || !bytes.Contains(raw, []byte("run_timeframes:")) {
		t.Fatal("generated config is not canonical strategy YAML")
	}
}
