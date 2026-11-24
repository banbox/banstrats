package adv

import (
	"cmp"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/entry"
	"github.com/spf13/cobra"
)

func init() {
	entry.AddGroup("chart", "generate chart commands")
	entry.AddCommandFactory("chart", newKlineChartCommand)
	entry.AddCommandFactory("chart", newDemoChartCommand)
}

func newKlineChartCommand() *cobra.Command {
	var args config.CmdArgs
	var symbol, tf, timeRange string
	const defaultSymbol, defaultTF, defaultRange = "BTC/USDT", "1d", "20241001-20250101"
	command := &cobra.Command{
		Use:   "kline",
		Short: "generate a kline chart with indicators",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return genKlineChart(&args, cmp.Or(symbol, defaultSymbol), cmp.Or(tf, defaultTF), cmp.Or(timeRange, defaultRange))
		},
	}
	flags := command.Flags()
	flags.StringVar(&symbol, "pairs", defaultSymbol, "symbol to chart")
	flags.StringVar(&tf, "timeframes", defaultTF, "chart timeframe")
	flags.StringVar(&timeRange, "timerange", defaultRange, "chart time range (start-end)")
	flags.StringVar(&args.DataDir, "datadir", "", "path to the data directory")
	flags.StringArrayVar((*[]string)(&args.Configs), "config", nil, "config path; may be repeated")
	flags.BoolVar(&args.NoDefault, "no-default", false, "ignore config.yml and config.local.yml")
	flags.StringVar(&args.ConfigData, "config-data", "", "inline YAML config")
	flags.StringVar(&args.Logfile, "logfile", "", "log file path")
	flags.StringVar(&args.LogLevel, "level", "info", "logging level")
	flags.IntVar(&args.MaxPoolSize, "max-pool-size", 0, "maximum database pool size")
	return command
}

func newDemoChartCommand() *cobra.Command {
	var exgName, market, symbol, tf, timeRange string
	command := &cobra.Command{
		Use:   "demo",
		Short: "generate a demo line chart",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return genAnyChart(exgName, market, symbol, tf, timeRange)
		},
	}
	flags := command.Flags()
	flags.StringVar(&exgName, "exchange", "binance", "exchange name")
	flags.StringVar(&market, "market", "spot", "market type: spot/linear/inverse")
	flags.StringVar(&symbol, "symbol", "BTC/USDT", "symbol to chart")
	flags.StringVar(&tf, "timeframe", "1h", "chart timeframe")
	flags.StringVar(&timeRange, "timerange", "20241201-20250101", "chart time range (start-end)")
	return command
}
