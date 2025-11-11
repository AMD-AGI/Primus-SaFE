package logs

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/regexUtil"
)

var (
	perfRegexps = map[string]*regexp.Regexp{
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
		"primus-hip-memory": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
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
)

func WorkloadLog(ctx context.Context, podUid string, msg string, logTime time.Time) error {
	log.Tracef("before consume workload log , pod uid %s", podUid)
	err := singleWorkloadLog(ctx, podUid, msg, logTime)
	if err != nil {
		log.GlobalLogger().WithError(err).Errorln("singleWorkloadLog error")
		err = nil
	}
	log.Tracef("workload log  consume success.before consume diagnosis")
	return nil
}

func singleWorkloadLog(ctx context.Context, podUid string, msg string, docTime time.Time) error {
	if hit, err := saveStartTrain(ctx, msg, podUid, docTime); hit {
		log.GlobalLogger().Debugf("start train, podUid: %s, ", podUid)
		IncLogAnalysisCount(constant.TrainingEventStartTrain)
		if err != nil {
			return err
		}
	} else if err != nil {
		log.Errorf("saveStartTrain err %+v", err)
	}
	if hit, err := saveTrainingPerformance(ctx, msg, podUid, docTime); hit {
		log.GlobalLogger().Debugf("Got training performance, podUid: %s, ", podUid)
		IncLogAnalysisCount(constant.TrainingPerformance)
		if err != nil {
			return err
		}
	} else if err != nil {
		log.Errorf("saveTrainingPerformance err %+v", err)
	}

	return nil
}

func saveTrainingPerformance(ctx context.Context, msg, podUid string, docTime time.Time) (bool, error) {
	performance, err := filterTrainingPerformance(msg)
	if err != nil {
		return false, err
	}
	if performance == nil {
		return false, nil
	}
	log.Tracef("save training performance, podUid: %s, peformance %+v ", podUid, performance)
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podUid)
	if err != nil {
		return true, err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}
	log.Tracef("nearest workload uid: %s", nearestWorkloadUid)

	workloadReferences, err := database.GetFacade().GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, []string{podUid})
	if err != nil {
		return true, err
	}
	log.Tracef("workload references %+v", workloadReferences)
	for _, reference := range workloadReferences {
		err = saveTrainingPerformanceForSingleWorkload(ctx, podUid, reference.WorkloadUID, nearestWorkloadUid, performance, docTime)
		if err != nil {
			log.Errorf("saveStartTrainForSingleWorkload err %+v", err)
		} else {
			log.Tracef("save training performance for workload %s", reference.WorkloadUID)
		}
	}
	return true, nil
}

func saveTrainingPerformanceForSingleWorkload(ctx context.Context, podId, workloadId, nearestWorkloadId string, perf *model.TrainingPerformance, docTime time.Time) error {
	serial, err := getCurrentRunSerial(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return err
	}
	existDbPerformance, err := database.GetFacade().GetTraining().GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, workloadId, serial, perf.CurrentIteration)
	if err != nil {
		return err
	}
	if existDbPerformance != nil {
		return nil
	}
	ext, _ := mapUtil.EncodeMap(perf)
	existDbPerformance = &dbModel.TrainingPerformance{
		ID:          0,
		PodUUID:     podId,
		Performance: ext,
		Iteration:   int32(perf.CurrentIteration),
		CreatedAt:   docTime,
		Serial:      int32(serial),
		WorkloadUID: workloadId,
	}
	err = database.GetFacade().GetTraining().CreateTrainingPerformance(ctx, existDbPerformance)
	if err != nil {
		return err
	}
	return nil
}

func filterTrainingPerformance(msg string) (*model.TrainingPerformance, error) {
	for name, reg := range perfRegexps {
		if !reg.MatchString(msg) {
			continue
		}
		if strings.Contains(msg, "rocm mem usage/free/total/usage_ratio") {
			log.Debugf("msg %s match regular expression %s", msg, name)
		}
		result := &model.TrainingPerformance{}
		err := regexUtil.RegexToStruct(reg, msg, result)
		if err != nil {
			log.Errorf("Regex match error for %s. %+v", name, err)
		} else {
			return result, nil
		}
	}
	return nil, nil
}

func saveStartTrain(ctx context.Context, msg, podId string, docTime time.Time) (bool, error) {
	if !strings.Contains(strings.TrimSpace(msg), "training ...") {
		return false, nil
	}
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podId)
	if err != nil {
		return false, err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}

	workloadReferences, err := database.GetFacade().GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, []string{podId})
	if err != nil {
		return true, err
	}
	for _, reference := range workloadReferences {
		err = saveStartTrainForSingleWorkload(ctx, podId, reference.WorkloadUID, nearestWorkloadUid, docTime)
		if err != nil {
			log.Errorf("saveStartTrainForSingleWorkload err %+v", err)
		}
	}
	return true, nil
}

func getCurrentRunSerial(ctx context.Context, workloadId, nearestWorkloadId string) (int, error) {
	existEvent, err := database.GetFacade().GetWorkload().GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx, workloadId, nearestWorkloadId, constant.TrainingEventStartTrain)
	if err != nil {
		return 0, err
	}
	if existEvent != nil {
		return 1, nil
	}
	serial := 1
	latestEvent, err := database.GetFacade().GetWorkload().GetLatestOtherWorkloadEvent(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return 0, err
	}
	if latestEvent != nil {
		serial = int(latestEvent.RunSerial + 1)
	}
	return serial, nil
}

func saveStartTrainForSingleWorkload(ctx context.Context, podId, workloadId, nearestWorkloadId string, docTime time.Time) error {
	serial, err := getCurrentRunSerial(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return err
	}
	newEvent := &dbModel.WorkloadEvent{
		WorkloadUID:        workloadId,
		Type:               constant.TrainingEventStartTrain,
		RunSerial:          int32(serial),
		CreatedAt:          docTime,
		PodUID:             podId,
		NearestWorkloadUID: nearestWorkloadId,
	}
	err = database.GetFacade().GetWorkload().CreateWorkloadEvent(ctx, newEvent)
	if err != nil {
		return err
	}
	return nil
}
