package main

import (
	"fmt"
	"regexp"
)

func main() {
	log := `[[32m20251121 12:10:29[0m][[36mrank-31/32[0m][[1mINFO [0m] [1m[--------trainer.py:2488] :  iteration    12125/   36000 | consumed samples:      3104000 | elapsed time per iteration (ms): 2701.3/2712.7 | hip mem usage/free/total/usage_ratio: 227.35GB/28.63GB/255.98GB/88.81% | throughput per GPU (TFLOP/s/GPU): 210.5/217.3 | tokens per GPU (tokens/s/GPU): 12130.4/12525.5 | learning rate: 1.587349E-04 | global batch size:   256 | lm loss: 2.352391E+00 | loss scale: 1.0 | grad norm: 0.132 | number of skipped iterations:   0 | number of nan iterations:   0 |[0m`
	perfRegexps := map[string]*regexp.Regexp{
		"primus-3": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
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
		"primus-hip-memory-v2": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
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
			`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
			`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*`),
	}

	// New version log (without num zeros field)
	logV2 := `0[[32m20251202 06:26:45[0m][[36mrank-7/8[0m][[1mINFO [0m] [1m[--------trainer.py:2560] : iteration 4273/ 5000 | consumed samples: 546944 | elapsed time per iteration (ms): 13107.0/13341.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 579.1/569.3 | tokens per GPU (tokens/s/GPU): 10000.2/9831.0 | learning rate: 5.130331E-07 | global batch size: 128 | lm loss: 4.092303E-03 | loss scale: 1.0 | grad norm: 0.003 | number of skipped iterations: 0 | number of nan iterations: 0 |[0m`

	fmt.Println("=== Test old version log (with num zeros field) ===")
	for name, r := range perfRegexps {
		if !r.MatchString(log) {
			continue
		}
		fmt.Printf("%s match\n", name)
	}

	fmt.Println("\n=== Test new version log (without num zeros field) ===")
	for name, r := range perfRegexps {
		if !r.MatchString(logV2) {
			continue
		}
		fmt.Printf("%s match\n", name)
	}
}
