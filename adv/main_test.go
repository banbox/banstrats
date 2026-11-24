package adv

import (
	"bytes"
	"strings"
	"testing"

	"github.com/banbox/banbot/entry"
)

func TestChartCommandFactories(t *testing.T) {
	first, _, err := entry.NewRootCommand().Find([]string{"chart", "kline"})
	if err != nil || first.Name() != "kline" {
		t.Fatalf("kline command was not registered: %v", err)
	}
	if err := first.Flags().Set("pairs", "ETH/USDT"); err != nil {
		t.Fatal(err)
	}
	second, _, err := entry.NewRootCommand().Find([]string{"chart", "kline"})
	if err != nil || second.Name() != "kline" {
		t.Fatalf("kline command was not registered on second root: %v", err)
	}
	if second == first || second.Flags().Changed("pairs") {
		t.Fatal("chart command state leaked between roots")
	}
}

func TestChartHelp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{"kline", []string{"pairs", "timeframes", "timerange", "datadir", "config", "no-default", "config-data", "logfile", "level", "max-pool-size"}},
		{"demo", []string{"exchange", "market", "symbol", "timeframe", "timerange"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := entry.NewRootCommand()
			cmd, _, err := root.Find([]string{"chart", tc.name})
			if err != nil || cmd.Name() != tc.name || cmd.DisableFlagParsing {
				t.Fatalf("Cobra chart command was not registered: %v", err)
			}
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs([]string{"chart", tc.name, "--help"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, flag := range tc.flags {
				if cmd.Flags().Lookup(flag) == nil || !strings.Contains(out.String(), "--"+flag+" ") {
					t.Errorf("help does not list --%s", flag)
				}
			}
		})
	}
}

func TestChartArgumentErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"kline range", []string{"kline", "-timerange=bad-range"}, "invalid chart time range"},
		{"demo range and legacy symbol", []string{"demo", "-symbol", "ETH/USDT", "--timerange=bad-range"}, "invalid chart time range"},
		{"demo unknown flag", []string{"demo", "--no-such-flag"}, "unknown flag"},
		{"demo positional", []string{"demo", "extra"}, "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := entry.Execute(append([]string{"chart"}, tc.args...))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}
