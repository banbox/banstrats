package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

/*
策略名称：ma:postApi
需在实盘时启用api_server，设置访问用户和一个强密码。

然后Post api http://127.0.0.1:8001/api/strat_call
body: {
  "token": "123", // 这是api_server中的密码
  "strategy": "ma:postApi", // 这是请求策略的名称
  "action": "openLong",
  "data": 123 // 其他任意需要发送到策略的数据
}
*/

func PostApi(p *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum: 100,
		OnBar: func(s *strat.StratJob) {
			// do nothing
		},
		OnPostApi: func(client *core.ApiClient, msg map[string]interface{}, jobs map[string]map[string]*strat.StratJob) error {
			action := utils.PopMapVal(msg, "action", "")
			for acc, pairMap := range jobs {
				for pairTF, job := range pairMap {
					if action == "openLong" {
						log.Info("open long from api", zap.String("acc", acc), zap.String("pairTF", pairTF))
						job.OpenOrder(&strat.EnterReq{
							Tag: "long",
						})
					} else {
						log.Warn("unknown action", zap.String("action", action))
					}
				}
			}
			return nil
		},
	}
}
