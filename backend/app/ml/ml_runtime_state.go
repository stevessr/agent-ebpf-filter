package ml

import "sync"

// MLRuntimeSnapshot is the atomically observed online ML state. Model training
// happens outside the lock; completed models and their metadata are published
// together so inference never sees a new engine with stale loaded/type flags.
type MLRuntimeSnapshot struct {
	Engine      Model
	Config      MLConfig
	Enabled     bool
	ModelLoaded bool
	ModelType   ModelType
}

var mlRuntimeStore struct {
	sync.RWMutex
	snapshot MLRuntimeSnapshot
}

func SnapshotMLRuntime() MLRuntimeSnapshot {
	mlRuntimeStore.RLock()
	snapshot := mlRuntimeStore.snapshot
	mlRuntimeStore.RUnlock()
	return snapshot
}

func ReplaceMLRuntime(snapshot MLRuntimeSnapshot) {
	snapshot.ModelLoaded = snapshot.ModelLoaded && snapshot.Engine != nil
	mlRuntimeStore.Lock()
	mlRuntimeStore.snapshot = snapshot
	mlRuntimeStore.Unlock()
	globalPredictionCache.Clear()
}

func UpdateMLRuntimeConfig(cfg MLConfig, enabled bool) {
	mlRuntimeStore.Lock()
	mlRuntimeStore.snapshot.Config = cfg
	mlRuntimeStore.snapshot.Enabled = enabled
	mlRuntimeStore.Unlock()
}

func PublishMLRuntimeModel(model Model, modelType ModelType) {
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
