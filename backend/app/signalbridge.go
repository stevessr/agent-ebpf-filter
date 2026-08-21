package app

// Bridge aliases onto package signalruntime (see backend/app/signalruntime).

import "agent-ebpf-filter/app/signalruntime"

var (
	normalizeSignalProcessingSettings = signalruntime.NormalizeSettings
	defaultSignalProcessingSettings   = signalruntime.DefaultSettings
	startSignalProcessingWorker       = signalruntime.StartProcessingWorker

	signalProcessingWorkerStore  = signalruntime.Worker()
	signalProgramLogWriterStore  = signalruntime.LogWriter()
	startSignalProgramLogWriter  = signalruntime.StartProgramLogWriter
	queueSignalProcessingRecord  = signalruntime.QueueProcessingRecord
	persistSignalProgramLog      = signalruntime.PersistProgramLog
	handleSignalProcessingStatus = signalruntime.HandleStatus
	handleSignalProcessingTask   = signalruntime.HandleTask
	handleSignalRuleTest         = signalruntime.HandleRuleTest
	handleSignalProgramLogs      = signalruntime.HandleProgramLogs

	handleSignalProgramLogDownload = signalruntime.HandleProgramLogDownload
)
