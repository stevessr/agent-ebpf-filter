package app

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
)

type mlAutoTuneParamsTask struct {
	Request MLAutoTuneRequest
}

type mlAutoTuneModelsTask struct {
	Request    MLModelTuneRequest
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
	jobID := entry.id
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("panic: %v", recovered)
			log.Printf("[ML] Auto-tune panic: %v", recovered)
			globalAutoTuneState.setError(jobID, message)
			err = newBackendTaskPanicError(recovered)
		}
	}()

	stopCancelWatch := make(chan struct{})
	cancelWatchDone := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer close(cancelWatchDone)
		select {
		case <-entry.cancel:
			globalTrainer.requestCancel()
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
		globalAutoTuneState.setError(jobID, err.Error())
		return err
	}
}

func runMLAutoTuneParamsTask(entry *backendTaskRuntimeEntry, jobID string, req MLAutoTuneRequest) error {
	cfg := currentMLConfig()
	log.Printf("[ML] Auto-tune started: jobID=%s, model=%s, grid=%dx%d, x=%s, y=%s",
		jobID, cfg.ModelType, req.GridSize, req.GridSize, req.XAxis, req.YAxis)
	resp, err := globalTrainer.AutoTuneWithConfig(globalTrainingStore, cfg, req, func(completed, total int, message string) {
		if total > 0 {
			entry.SetProgress(float64(completed) / float64(total))
		}
		if completed%5 == 0 || completed == total {
			log.Printf("[ML] Auto-tune progress: %d/%d — %s", completed, total, message)
		}
		globalAutoTuneState.update(jobID, completed, total, message)
	})
	if err != nil {
		log.Printf("[ML] Auto-tune error: %v", err)
	} else {
		log.Printf("[ML] Auto-tune done: %d cells, best score=%.4f", len(resp.Cells), autoTuneBestScore(resp))
	}
	globalAutoTuneState.finish(jobID, resp, err)
	return normalizeMLAutoTuneTaskError(entry, err)
}

func runMLAutoTuneModelsTask(entry *backendTaskRuntimeEntry, jobID string, payload mlAutoTuneModelsTask) error {
	resp, err := runModelAutoTuneWithCancel(globalTrainingStore, payload.Request, payload.ModelTypes, func(completed, total int, message string) {
		if total > 0 {
			entry.SetProgress(float64(completed) / float64(total))
		}
		globalAutoTuneState.update(jobID, completed, total, message)
	}, entry.IsCanceled)
	if err != nil {
		log.Printf("[ML] Model auto-tune error: %v", err)
	} else {
		best := ""
		if resp.Best != nil {
			best = resp.Best.ModelType
		}
		log.Printf("[ML] Model auto-tune done: %d candidates, best=%s", len(resp.Candidates), best)
	}
	globalAutoTuneState.finishModelTune(jobID, resp, err)
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
	state := globalAutoTuneState.snapshot()
	if state.JobID != "" {
		mlAutoTuneTasks.Cancel(state.JobID)
	}
	globalTrainer.requestCancel()
	if state.Running {
		globalAutoTuneState.setError(state.JobID, "cancelled")
	}
}
