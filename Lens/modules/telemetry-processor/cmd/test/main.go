package main

import (
	"fmt"
	"regexp"
)

func main() {
	log := `[[32m20251023 12:39:12[0m][[36mrank-63/64[0m][[1mINFO [0m] [1m[--------trainer.py:2382] :  iteration      424/   40000 | consumed samples:        27136 | elapsed time per iteration (ms): 16601.3/17219.3 | hip mem usage/free/total/usage_ratio: 253.38GB/2.60GB/255.98GB/98.98% | throughput per GPU (TFLOP/s/GPU): 11.9/11.5 | tokens per GPU (tokens/s/GPU): 246.7/238.1 | learning rate: 9.997254E-06 | global batch size:    64 | lm loss: 4.447942E+00 | loss scale: 1.0 | grad norm: 6.142 | num zeros: 960902016.0 | number of skipped iterations:   0 | number of nan iterations:   0 |[0m`
	perfRegexps := map[string]*regexp.Regexp{
		"primus-legancy": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+mem\s+usages:\s+(?P<MemUsages>\d+\.\d+)\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
		"primus": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
			`\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|` +
			`\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+rocm\s+mem\s+usage/free/total/usage_ratio:\s+` +
			`(?P<MemUsage>\d+\.\d+)GB/` +
			`(?P<MemFree>\d+\.\d+)GB/` +
			`(?P<MemTotal>\d+\.\d+)GB/` +
			`(?P<MemUsageRatio>\d+\.\d+)%\s+\|` +
			`\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|` +
			`\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|` +
			`\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|` +
			`\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|` +
			`\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|` +
			`\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|` +
			`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
			`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
		"primus-2": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
			`\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|` +
			`\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+` +
			`(?P<MemUsage>\d+\.\d+)GB/` +
			`(?P<MemFree>\d+\.\d+)GB/` +
			`(?P<MemTotal>\d+\.\d+)GB/` +
			`(?P<MemUsageRatio>\d+\.\d+)%\s+\|` +
			`\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|` +
			`\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|` +
			`\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|` +
			`\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|` +
			`\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|` +
			`\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|` +
			`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
			`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
	}

	for name, r := range perfRegexps {
		if !r.MatchString(log) {
			continue
		}
		fmt.Println(fmt.Sprintf("%s match", name))
	}
}
