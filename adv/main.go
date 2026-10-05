package adv

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/entry"
	"github.com/spf13/cobra"
)

func init() {
	entry.AddGroup("chart", "generate chart commands")

	entry.AddCommandFactory("chart", func() *cobra.Command {
		args := &config.CmdArgs{}
		command := &cobra.Command{
			Use: "kline", Short: "generate klineChart for symbol", Args: cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				if err := genKlineChart(args); err != nil {
					return err
				}
				return nil
			},
		}
		command.Flags().StringVar(&args.RawPairs, "pairs", "", "symbol to chart")
		command.Flags().StringVar(&args.RawTimeFrames, "timeframes", "", "chart timeframe")
		return command
	})

	entry.AddCommandFactory("chart", func() *cobra.Command {
		return &cobra.Command{
			Use: "demo", Short: "generate demoChart", DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error { return genAnyChart(args) },
		}
	})
}
