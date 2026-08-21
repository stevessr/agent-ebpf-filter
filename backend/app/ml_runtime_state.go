package app

import "sync"

// mlRuntimeSnapshot is the atomically observed online ML state. Model training
// happens outside the lock; completed models and their metadata are published
// together so inference never sees a new engine with stale loaded/type flags.
type mlRuntimeSnapshot struct {
	Engine      Model
	Config      MLConfig
	Enabled     bool
	ModelLoaded bool
	ModelType   ModelType
}

var mlRuntimeStore struct {
	sync.RWMutex
	snapshot mlRuntimeSnapshot
}

func snapshotMLRuntime() mlRuntimeSnapshot {
	mlRuntimeStore.RLock()
	snapshot := mlRuntimeStore.snapshot
	mlRuntimeStore.RUnlock()
	return snapshot
}

func replaceMLRuntime(snapshot mlRuntimeSnapshot) {
	snapshot.ModelLoaded = snapshot.ModelLoaded && snapshot.Engine != nil
	mlRuntimeStore.Lock()
	mlRuntimeStore.snapshot = snapshot
	mlRuntimeStore.Unlock()
	globalPredictionCache.Clear()
}

func updateMLRuntimeConfig(cfg MLConfig, enabled bool) {
	mlRuntimeStore.Lock()
	mlRuntimeStore.snapshot.Config = cfg
	mlRuntimeStore.snapshot.Enabled = enabled
	mlRuntimeStore.Unlock()
}

func publishMLRuntimeModel(model Model, modelType ModelType) {
	if modelType == "" && model != nil {
		modelType = model.Type()
	}
	mlRuntimeStore.Lock()
	mlRuntimeStore.snapshot.Engine = model
	mlRuntimeStore.snapshot.ModelLoaded = model != nil
	if modelType != "" {
		mlRuntimeStore.snapshot.ModelType = modelType
	}
	mlRuntimeStore.Unlock()
	globalPredictionCache.Clear()
}
