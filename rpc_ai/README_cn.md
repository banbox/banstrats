rpc_ai是通过grpc与python端ML/DL结合使用的一个示例。

代码文件简介：
* fea_base: 对按传入参数对`biz.RunHistKline`进行包装，提供带未来K线的OnBar回调
* fea_gen: 基于fea_base，生成AI特征通过grpc返回
* fea_test: go端的grpc客户端，可与`bot tool data_server`进行通信，验证grpc接口返回数据是否正常。
* trader1: 基于AI模型的交易策略，通过grpc调用python端的grpc模型服务，执行开平仓等动作。

# 如何开始
## 1. 注册数据采样器
向`biz.FeaGenerators`中添加自定义的特征采样器，目前已添加`pubAiFea`。
可在每个bar中添加关键信息Debug记录，在`OnEnvEnd`中保存到文件，然后调用`TestDataClient`触发
## 2. 实现策略基础部分
向`strat.AddStratGroup`注册策略，使用同样的特征采样。
同样记录关键信息到`s.Stagy.Outputs`中，回测完成自动保存到文件。对比和1中的文件是否一致。
## 3. 生成行情预测数据
go端启动`bot tool data_server`，然后python端编写调用grpc的代码，获得数据并落盘为训练数据文件。
## 4. 训练行情模型
示例：将行情作为三分类任务：不确定、上升、下降；
## 5. 部署模型服务

在策略配置中指定模型服务地址。`rpc_ai:trade1` 不再使用硬编码地址；未配置服务时会安全跳过推理。

```yaml
run_policy:
  - name: rpc_ai:trade1
    ai_grpc_addr: "127.0.0.1:8080"
    ai_grpc_timeout_ms: 5000
```

## 6. 完善交易策略
修改第二步的策略，将数据通过grpc请求部署的模型，根据返回结果进行交易
