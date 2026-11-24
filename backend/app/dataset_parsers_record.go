package app

import (
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"
	"strings"
	"time"
)

func remoteDatasetRecordFromAny(decoded any, rowIndex int, source string) (remoteDatasetRecord, bool) {
	switch value := decoded.(type) {
	case string:
		if record, ok := selinuxPolicyRuleRecordFromLine(value, rowIndex, source); ok {
			return record, true
		}
		comm, args := behavior.NormalizeCommandInput(value, "", nil)
		if comm == "" {
			return remoteDatasetRecord{}, false
		}
		return remoteDatasetRecord{
			Row:         rowIndex,
			Source:      source,
			CommandLine: value,
			Comm:        comm,
			Args:        args,
			UserLabel:   "remote-import",
		}, true
	case map[string]any:
		return remoteDatasetRecordFromMap(value, rowIndex, source)
	default:
		return remoteDatasetRecord{}, false
	}
}

func remoteDatasetRecordFromMap(row map[string]any, rowIndex int, source string) (remoteDatasetRecord, bool) {
	record := remoteDatasetRecord{Row: rowIndex, Source: source, UserLabel: "remote-import"}

	commandLine := firstStringValue(row, "commandLine", "commandline", "cmdline", "full_command", "command", "shell", "payload", "text", "value", "Command", "code", "rule", "policy", "selinuxRule", "selinuxrule", "selinux_rule")
	if selinuxRecord, ok := selinuxPolicyRuleRecordFromLine(commandLine, rowIndex, source); ok {
		label := normalizeDatasetLabelValue(row["label"])
		if label == "" {
			label = normalizeDatasetLabelValue(row["action"])
		}
		if label == "" {
			label = normalizeDatasetLabelValue(row["class"])
		}
		if label != "" {
			selinuxRecord.Label = label
			selinuxRecord.LabelSource = "dataset"
		}
		if category := firstStringValue(row, "category", "behavior", "type", "group", "Category", "_injected_category"); category != "" {
			selinuxRecord.Category = category
		}
		if anomaly, ok := extractDatasetFloat(row, "anomalyScore", "anomaly_score", "score", "riskScore"); ok {
			selinuxRecord.Anomaly = anomaly
			selinuxRecord.HasAnomaly = true
		}
		if ts, ok := extractDatasetTimestamp(row); ok {
			selinuxRecord.Timestamp = ts
		}
		if userLabel := firstStringValue(row, "userLabel", "userlabel", "user_label"); userLabel != "" {
			selinuxRecord.UserLabel = userLabel
		}
		return selinuxRecord, true
	}
	comm := firstStringValue(row, "comm", "commandName", "commandname", "name", "executable", "Name", "_injected_name")
	args := extractDatasetArgs(row, commandLine)
	if commandLine == "" && comm != "" {
		commandLine = behavior.JoinCommandLine(comm, args)
	}
	if commandLine != "" && comm == "" {
		comm, args = behavior.NormalizeCommandInput(commandLine, "", nil)
	}
	if comm == "" && commandLine == "" {
		return remoteDatasetRecord{}, false
	}

	record.CommandLine = commandLine
	record.Comm = comm
	record.Args = args
	record.Label = normalizeDatasetLabelValue(row["label"])
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["action"])
	}
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["class"])
	}
	if record.Label != "" {
		record.LabelSource = "dataset"
	}

	record.Category = firstStringValue(row, "category", "behavior", "type", "group", "Category", "_injected_category")
	if anomaly, ok := extractDatasetFloat(row, "anomalyScore", "anomaly_score", "score", "riskScore"); ok {
		record.Anomaly = anomaly
		record.HasAnomaly = true
	}
	if ts, ok := extractDatasetTimestamp(row); ok {
		record.Timestamp = ts
	}
	if userLabel := firstStringValue(row, "userLabel", "userlabel", "user_label"); userLabel != "" {
		record.UserLabel = userLabel
	}

	return record, true
}

func buildRemoteDatasetRow(record remoteDatasetRecord, mode string, cleanSensitive bool) remoteDatasetRow {
	return buildRemoteDatasetRowWithIdentity(record, mode, cleanSensitive, newDatasetIdentityMapper())
}

func buildRemoteDatasetRowWithIdentity(record remoteDatasetRecord, mode string, cleanSensitive bool, ids *datasetIdentityMapper) remoteDatasetRow {
	if cleanSensitive {
		record = sanitizeRemoteDatasetRecordWithIdentity(record, ids)
	}
	comm, args := behavior.NormalizeCommandInput(record.CommandLine, record.Comm, record.Args)
	label := record.Label
	labelSource := record.LabelSource
	if label == "" {
		if inferredLabel, inferredSource := inferRemoteDatasetLabelFromSource(record.Source); inferredLabel != "" && strings.EqualFold(strings.TrimSpace(mode), "preserve") {
			label = inferredLabel
			labelSource = inferredSource
		}
	}
	if label == "" {
		label = "-"
	}
	if strings.EqualFold(strings.TrimSpace(mode), "block") {
		label = "BLOCK"
		labelSource = "forced"
	} else if strings.EqualFold(strings.TrimSpace(mode), "unlabeled") {
		label = "-"
		labelSource = "forced"
	} else if labelSource == "" {
		labelSource = "inferred"
	}

	category := record.Category
	if category == "" {
		category = behavior.ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := record.Anomaly
	if !record.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	timestamp := record.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	commandLine := strings.TrimSpace(record.CommandLine)
	if commandLine == "" {
		commandLine = behavior.JoinCommandLine(comm, args)
	}

	return remoteDatasetRow{
		Row:          record.Row,
		Source:       record.Source,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         args,
		Label:        label,
		LabelSource:  labelSource,
		Category:     category,
		AnomalyScore: anomaly,
		HasAnomaly:   record.HasAnomaly,
		Timestamp:    timestamp.Format(time.RFC3339),
		UserLabel:    record.UserLabel,
	}
}

func buildRemoteDatasetSample(row remoteDatasetRow, mode string, cleanSensitive bool) ml.TrainingSample {
	return buildRemoteDatasetSampleWithExtractor(row, mode, cleanSensitive, globalFeatureExtractor)
}

func buildRemoteDatasetSampleWithExtractor(row remoteDatasetRow, mode string, cleanSensitive bool, extractor *FeatureExtractor) ml.TrainingSample {
	if extractor == nil {
		extractor = globalFeatureExtractor
	}
	if cleanSensitive {
		row = sanitizeRemoteDatasetRow(row)
	}
	comm, args := behavior.NormalizeCommandInput(row.CommandLine, row.Comm, row.Args)
	timestamp := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, row.Timestamp); err == nil {
		timestamp = parsed.UTC()
	}

	category := row.Category
	if category == "" {
		category = behavior.ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := row.AnomalyScore
	if !row.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	label := int32(-1)
	userLabel := strings.TrimSpace(row.UserLabel)
	if userLabel == "" {
		userLabel = "remote-import"
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "block":
		label = ml.ActionFromLabel("BLOCK")
		userLabel = "remote-block"
	case "unlabeled":
		userLabel = "remote-import-unlabeled"
	default:
		if normalized := normalizeActionLabel(row.Label); normalized != "" {
			label = ml.ActionFromLabel(normalized)
			if userLabel == "remote-import" {
				userLabel = "remote-source-label"
			}
		} else if inferredLabel, inferredSource := inferRemoteDatasetLabelFromSource(row.Source); inferredLabel != "" {
			label = ml.ActionFromLabel(inferredLabel)
			if inferredSource != "" && userLabel == "remote-import" {
				userLabel = "remote-source-label"
			}
		} else if strings.EqualFold(strings.TrimSpace(mode), "heuristic") {
			assessment := assessCommandSafetyWithOptions(nil, comm, args, "", 0, commandSafetyAssessmentOptions{IncludeLLM: false})
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = ml.ActionFromLabel(action)
				if userLabel == "remote-import" {
					userLabel = "remote-heuristic"
				}
			}
		}
	}

	features := extractor.Extract(comm, args, "", 0)
	commandLine := strings.TrimSpace(row.CommandLine)
	if commandLine == "" {
		commandLine = behavior.JoinCommandLine(comm, args)
	}
	return ml.TrainingSample{
		Features:     features,
		Label:        label,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         args,
		Category:     category,
		AnomalyScore: anomaly,
		Timestamp:    timestamp,
		UserLabel:    userLabel,
	}
}
