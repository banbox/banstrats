#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BANBOT_DIR="${ROOT_DIR}/../banbot"

if [[ ! -d "${BANBOT_DIR}" ]]; then
  echo "expected sibling banbot repo at ${BANBOT_DIR}" >&2
  exit 1
fi

run_layer() {
  local label="$1"
  shift
  printf '\n==> %s\n' "$label"
  printf ':: %s\n' "$*"
  "$@"
}

run_in_banbot() {
  local label="$1"
  shift
  printf '\n==> %s\n' "$label"
  printf ':: (cd %s && %s)\n' "$BANBOT_DIR" "$*"
  (
    cd "$BANBOT_DIR"
    "$@"
  )
}

example_regex='^(TestMainBlankImportRegistersBinanceLongShortSource|TestBinanceLongShortSourceInfo|TestBinanceLongShortRegistersSource|TestBinanceLongShortFetchHistory|TestBinanceLongShortSubscribeLive|TestBinanceLongShortStrategyRegistersAndCollectsSubs|TestBinanceLongShortStrategyOnDataAndDataHubProof|TestBinanceLongShortStrategyIgnoresUnrelatedSource|TestBinanceLongShortStrategyReportsMissingDataHub|TestBinanceLongShortStrategyReportsMalformedValues|TestBinanceLongShortDataHubWindowBoundedAndLatestReadable|TestBinanceLongShortDataHubIgnoresLegacyKlineEntriesWithSameSidAndTf)$'

framework_regex='^(TestRegisterDataSourceRejectsDuplicates|TestRegisterDataSourceDoesNotSubscribeLive|TestActivateDataSourcesGroupsSelectedSubsBySource|TestCollectRuntimeDataSubsFiltersKlinesAndDeduplicates|TestEnsureThirdPartySeriesRangeUnknownSource|TestEnsureThirdPartySeriesRangePropagatesFetchFailure|TestEnsureThirdPartySeriesRangeRejectsSourceMetadataMismatch|TestEnsureThirdPartySeriesRangePropagatesRepoFailure|TestRegisteredSourceLookupsStayActivationFree|TestCryptoTraderEmitRoutesThirdPartyRowsThroughOnData|TestCryptoTraderRunEnsuresThirdPartyBeforeActivateAndLoop|TestCryptoTraderRunEnsureFailureStopsBeforeActivateAndLoop|TestCryptoTraderRunActivateFailureStopsBeforeLoop|TestCryptoTraderRunStartupActivatesSelectedSourcesOnce|TestCryptoTraderRunStartupReturnsUnknownSourceError|TestCryptoTraderRunStartupReturnsCallbackError|TestBackTestInitEnsuresThirdPartyBeforeLoop|TestBackTestEnsureThirdPartyRangeUsesWarmupDepthAndRunWindow|TestBackTestEnsureFailureStopsBeforeLoop|TestBackTestBootstrapRangeSkipsWhenNoThirdPartySubs|TestBackTestCollectRuntimeSubsExcludeKlineWarmPath|TestBackTestEnsureThirdPartyUsesLoadedJobs)$'

legacy_regex='^(TestFeedSeriesRoutesNonKlineDataSubs|TestFeedSeriesFallsBackToOnInfoBarForLegacyKlineSubs|TestFeedSeriesCoexistsForThirdPartyAndLegacyInfoSubs|TestDataHubLatestAndWindow|TestDataHubLatestAndWindowSeparatesThirdPartyAndLegacySeriesBySource|TestCollectDataSubsBridgesLegacyPairInfos|TestUpdatePairs_RebuildsWarmsFromCurrentDataSubs)$'

broad_regex='^(TestRegisterDataSourceRejectsDuplicates|TestRegisterDataSourceRejectsNil|TestRegisterDataSourceRejectsInvalidInfo|TestGetAndListDataSources|TestRegisterDataSourceDoesNotSubscribeLive|TestActivateDataSourcesGroupsSelectedSubsBySource|TestActivateDataSourcesFailsForUnknownSource|TestActivateDataSourcesRequiresSink|TestCollectRuntimeDataSubsFiltersKlinesAndDeduplicates|TestCollectRuntimeDataSubsRejectsMalformedSubs|TestEnsureThirdPartySeriesRangeDeduplicatesAndUsesCoverage|TestEnsureThirdPartySeriesRangeUnknownSource|TestEnsureThirdPartySeriesRangePropagatesFetchFailure|TestEnsureThirdPartySeriesRangeRejectsSourceMetadataMismatch|TestEnsureThirdPartySeriesRangePropagatesRepoFailure|TestRegisteredSourceLookupsStayActivationFree|TestCryptoTraderEmitRequiresStartupProvider|TestCryptoTraderEmitRoutesThirdPartyRowsThroughOnData|TestCryptoTraderRunSkipsStartupWhenCallbackNil|TestCryptoTraderRunEnsuresThirdPartyBeforeActivateAndLoop|TestCryptoTraderRunEnsureFailureStopsBeforeActivateAndLoop|TestCryptoTraderRunActivateFailureStopsBeforeLoop|TestCryptoTraderRunStartupActivatesSelectedSourcesOnce|TestCryptoTraderRunStartupReturnsUnknownSourceError|TestCryptoTraderRunStartupReturnsCallbackError|TestBackTestInitEnsuresThirdPartyBeforeLoop|TestBackTestEnsureThirdPartyRangeUsesWarmupDepthAndRunWindow|TestBackTestEnsureFailureStopsBeforeLoop|TestBackTestBootstrapRangeSkipsWhenNoThirdPartySubs|TestBackTestCollectRuntimeSubsExcludeKlineWarmPath|TestBackTestEnsureThirdPartyUsesLoadedJobs|TestFeedSeriesRoutesNonKlineDataSubs|TestFeedSeriesFallsBackToOnInfoBarForLegacyKlineSubs|TestFeedSeriesCoexistsForThirdPartyAndLegacyInfoSubs|TestCollectDataSubsNormalizesThirdPartySource|TestDataHubLatestAndWindow|TestDataHubLatestAndWindowSeparatesThirdPartyAndLegacySeriesBySource|TestCollectDataSubsBridgesLegacyPairInfos|TestUpdatePairs_RebuildsWarmsFromCurrentDataSubs)$'

cd "$ROOT_DIR"

run_layer \
  "Layer 1/4: banstrats example registration, fetch, strategy, and DataHub proof" \
  go test . ./longshort -run "$example_regex"

run_in_banbot \
  "Layer 2/4: banbot framework bootstrap, ensure, activation, and startup ordering proof" \
  go test ./data ./live ./opt -run "$framework_regex"

run_in_banbot \
  "Layer 3/4: banbot legacy OHLCV compatibility and DataHub source scoping proof" \
  go test ./biz ./strat -run "$legacy_regex"

run_in_banbot \
  "Layer 4/4: broad hermetic framework package sweep across targeted packages" \
  go test ./data ./live ./opt ./biz ./strat -run "$broad_regex"

printf '\nAll third-party regression proof layers passed.\n'
