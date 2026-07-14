[中文](README_cn.md)

rpc_ai is an example of integrating ML/DL with Python through gRPC.

Code File Overview:
* fea_base: Wraps `biz.RunHistKline` based on input parameters, providing OnBar callbacks with future candlesticks
* fea_gen: Generates AI features and returns them via gRPC based on fea_base
* fea_test: gRPC client in Go that can communicate with `bot tool data_server` to verify if gRPC interface returns data correctly
* trader1: AI model-based trading strategy that calls Python's gRPC model service through gRPC to execute open/close positions and other actions

# Getting Started
## 1. Register Data Sampler
Add custom feature samplers to `biz.FeaGenerators`, currently `pubAiFea` has been added.
You can add key information debug records in each bar, save them to file in `OnEnvEnd`, then trigger using `TestDataClient`

## 2. Implement Strategy Basics
Register strategy with `strat.AddStratGroup` using the same feature sampling.
Similarly record key information to `s.Stagy.Outputs`, which automatically saves to file when backtesting completes. Compare with the file from step 1 to verify consistency.

## 3. Generate Market Prediction Data
Start `bot tool data_server` on Go side, then write gRPC client code in Python to obtain data and save as training data files.

## 4. Train Market Model
Example: Treat market prediction as a three-class classification task: uncertain, rising, falling

## 5. Deploy Model Service

Configure the model endpoint on the strategy policy. `rpc_ai:trade1` does not
use a hard-coded host and skips inference safely when no endpoint is provided.

```yaml
run_policy:
  - name: rpc_ai:trade1
    ai_grpc_addr: "127.0.0.1:8080"
    ai_grpc_timeout_ms: 5000
```

## 6. Refine Trading Strategy
Modify the strategy from step 2 to request data from the deployed model via gRPC and trade based on the returned results
