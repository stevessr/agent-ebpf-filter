package app

import (
	"agent-ebpf-filter/app/ml"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
)

type mlAutoTuneParamsTask struct {
	Request ml.MLAutoTuneRequest
}

type mlAutoTuneModelsTask struct {
	Request    ml.MLModelTuneRequest
	ModelTypes []ModelType
}

const mlAutoTuneTaskQueueSize = 1

var mlAutoTuneTasks = newBackendTaskRuntime("ml-auto-tune", 32, runMLAutoTuneTask)

func startMLAutoTuneTasks() {
	mlAutoTuneTasks.Start(mlAutoTuneTaskQueueSize)
}

func runMLAutoTuneTask(entry *backendTaskRuntimeEntry) (err error) {
	if entry == nil {
		return errors.New("ML auto-tune task is unavailable")
	}
	jobID := entry.ID()
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("panic: %v", recovered)
			log.Printf("[ML] Auto-tune panic: %v", recovered)
			ml.GlobalAutoTuneState.SetError(jobID, message)
			err = newBackendTaskPanicError(recovered)
		}
	}()

	stopCancelWatch := make(chan struct{})
	cancelWatchDone := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer close(cancelWatchDone)
		select {
		case <-entry.CancelChan():
			ml.GlobalTrainer.RequestCancel()
		case <-stopCancelWatch:
		}
	}()
	defer func() {
		stopOnce.Do(func() { close(stopCancelWatch) })
		<-cancelWatchDone
	}()

	switch payload := entry.Payload().(type) {
	case mlAutoTuneParamsTask:
		return runMLAutoTuneParamsTask(entry, jobID, payload.Request)
	case mlAutoTuneModelsTask:
		return runMLAutoTuneModelsTask(entry, jobID, payload)
	default:
		err := fmt.Errorf("unsupported ML auto-tune payload %T", payload)
		ml.GlobalAutoTuneState.SetError(jobID, err.Error())
		return err
	}
}

func runMLAutoTuneParamsTask(entry *backendTaskRuntimeEntry, jobID string, req ml.MLAutoTuneRequest) error {
	cfg := currentMLConfig()
	log.Printf("[ML] Auto-tune started: jobID=%s, model=%s, grid=%dx%d, x=%s, y=%s",
		jobID, cfg.ModelType, req.GridSize, req.GridSize, req.XAxis, req.YAxis)
	resp, err := ml.GlobalTrainer.AutoTuneWithConfig(ml.GlobalTrainingStore, cfg, req, func(completed, total int, message string) {
		if total > 0 {
			entry.SetProgress(float64(completed) / float64(total))
		}
		if completed%5 == 0 || completed == total {
			log.Printf("[ML] Auto-tune progress: %d/%d — %s", completed, total, message)
		}
		ml.GlobalAutoTuneState.Update(jobID, completed, total, message)
	})
	if err != nil {
		log.Printf("[ML] Auto-tune error: %v", err)
	} else {
		log.Printf("[ML] Auto-tune done: %d cells, best score=%.4f", len(resp.Cells), ml.AutoTuneBestScore(resp))
	}
	ml.GlobalAutoTuneState.Finish(jobID, resp, err)
	return normalizeMLAutoTuneTaskError(entry, err)
}

func runMLAutoTuneModelsTask(entry *backendTaskRuntimeEntry, jobID string, payload mlAutoTuneModelsTask) error {
	resp, err := runModelAutoTuneWithCancel(ml.GlobalTrainingStore, payload.Request, payload.ModelTypes, func(completed, total int, message string) {
		if total > 0 {
			entry.SetProgress(float64(completed) / float64(total))
		}
		ml.GlobalAutoTuneState.Update(jobID, completed, total, message)
	}, entry.IsCanceled)
	if err != nil {
		log.Printf("[ML] ml.Model auto-tune error: %v", err)
	} else {
		best := ""
		if resp.Best != nil {
			best = resp.Best.ModelType
		}
		log.Printf("[ML] ml.Model auto-tune done: %d candidates, best=%s", len(resp.Candidates), best)
	}
	ml.GlobalAutoTuneState.FinishModelTune(jobID, resp, err)
	return normalizeMLAutoTuneTaskError(entry, err)
}

func normalizeMLAutoTuneTaskError(entry *backendTaskRuntimeEntry, err error) error {
	if entry != nil && entry.IsCanceled() {
		return errBackendTaskCanceled
	}
	if err != nil && strings.EqualFold(strings.TrimSpace(err.Error()), "cancelled") {
		return errBackendTaskCanceled
	}
	return err
}

func cancelMLAutoTuneTasks() {
	state := ml.GlobalAutoTuneState.Snapshot()
	if state.JobID != "" {
		mlAutoTuneTasks.Cancel(state.JobID)
	}
	ml.GlobalTrainer.RequestCancel()
	if state.Running {
		ml.GlobalAutoTuneState.SetError(state.JobID, "cancelled")
	}
}
