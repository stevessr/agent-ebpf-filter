package ml

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/behavior"
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const trainingStoreMaxWireStringBytes = 1<<16 - 1

var (
	errTrainingStoreChangedDuringLoad = errors.New("training store changed while loading persisted data")
	errTrainingStorePathOutsideRoot   = errors.New("training store path must be a file directly under its data directory")
	errTrainingStoreTooLarge          = errors.New("training store exceeds the persistence size limit")
)

type trainingStoreLimitWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *trainingStoreLimitWriter) Write(payload []byte) (int, error) {
	if int64(len(payload)) > w.remaining {
		return 0, errTrainingStoreTooLarge
	}
	written, err := w.writer.Write(payload)
	w.remaining -= int64(written)
	if err == nil && written != len(payload) {
		err = io.ErrShortWrite
	}
	return written, err
}

func persistTrainingStoreSnapshot(dataDir, persistPath string, samples []TrainingSample) error {
	count := 0
	for _, sample := range samples {
		if !sample.Timestamp.IsZero() {
			count++
		}
	}
	if count > trainingStoreMaxSamples {
		return fmt.Errorf("persist training store: sample count %d exceeds limit %d", count, trainingStoreMaxSamples)
	}

	rootPath, targetName, err := resolveTrainingStoreTarget(dataDir, persistPath)
	if err != nil {
		return err
	}
	root, err := openTrainingStoreRoot(rootPath)
	if err != nil {
		return fmt.Errorf("open training store directory: %w", err)
	}
	defer root.Close()

	file, tempName, err := platform.CreateTempSibling(root, "ml-training-data")
	if err != nil {
		return fmt.Errorf("create training store temporary file: %w", err)
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = unix.Unlinkat(int(root.Fd()), tempName, 0)
		}
	}()

	limited := &trainingStoreLimitWriter{writer: file, remaining: trainingStoreMaxFileBytes}
	buffered := bufio.NewWriterSize(limited, 64*1024)
	if err := writeTrainingStore(buffered, samples, count); err != nil {
		return err
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush training store snapshot: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync training store snapshot: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close training store snapshot: %w", err)
	}
	if err := platform.ReplaceFileInDir(root, tempName, targetName); err != nil {
		return fmt.Errorf("replace training store snapshot: %w", err)
	}
	cleanup = false
	return nil
}

func writeTrainingStore(writer io.Writer, samples []TrainingSample, count int) error {
	if err := writeTrainingBytes(writer, []byte(trainingStoreMagic)); err != nil {
		return fmt.Errorf("write training store magic: %w", err)
	}
	var integer [4]byte
	binary.LittleEndian.PutUint32(integer[:], uint32(count))
	if err := writeTrainingBytes(writer, integer[:]); err != nil {
		return fmt.Errorf("write training store count: %w", err)
	}

	written := 0
	for _, rawSample := range samples {
		if rawSample.Timestamp.IsZero() {
			continue
		}
		sample := NormalizeTrainingSample(rawSample)
		argsJSON, err := json.Marshal(sample.Args)
		if err != nil {
			return fmt.Errorf("encode training sample %d arguments: %w", written, err)
		}
		if len(argsJSON) > trainingSampleMaxArgsJSONBytes {
			return fmt.Errorf("encode training sample %d arguments: encoded value exceeds %d bytes", written, trainingSampleMaxArgsJSONBytes)
		}
		if err := validateTrainingStoreField("command line", sample.CommandLine, trainingSampleMaxCommandLineBytes); err != nil {
			return fmt.Errorf("encode training sample %d: %w", written, err)
		}
		if err := validateTrainingStoreField("command", sample.Comm, trainingSampleMaxCommBytes); err != nil {
			return fmt.Errorf("encode training sample %d: %w", written, err)
		}

		var fixed [20]byte
		binary.LittleEndian.PutUint64(fixed[0:8], uint64(sample.Timestamp.UnixNano()))
		binary.LittleEndian.PutUint32(fixed[8:12], uint32(sample.Label))
		binary.LittleEndian.PutUint64(fixed[12:20], math.Float64bits(sample.AnomalyScore))
		if err := writeTrainingBytes(writer, fixed[:]); err != nil {
			return fmt.Errorf("write training sample %d metadata: %w", written, err)
		}
		if err := writeTrainingStoreString(writer, sample.CommandLine); err != nil {
			return fmt.Errorf("write training sample %d command line: %w", written, err)
		}
		if err := writeTrainingStoreString(writer, sample.Comm); err != nil {
			return fmt.Errorf("write training sample %d command: %w", written, err)
		}
		if err := writeTrainingStorePayload(writer, argsJSON); err != nil {
			return fmt.Errorf("write training sample %d arguments: %w", written, err)
		}

		var features [FeatureDim * 8]byte
		for index, value := range sample.Features {
			binary.LittleEndian.PutUint64(features[index*8:(index+1)*8], math.Float64bits(value))
		}
		if err := writeTrainingBytes(writer, features[:]); err != nil {
			return fmt.Errorf("write training sample %d features: %w", written, err)
		}
		written++
	}
	if written != count {
		return fmt.Errorf("write training store: counted %d samples but encoded %d", count, written)
	}
	return nil
}

func validateTrainingStoreField(name, value string, configuredLimit int) error {
	if len(value) > configuredLimit {
		return fmt.Errorf("%s exceeds %d bytes", name, configuredLimit)
	}
	if len(value) > trainingStoreMaxWireStringBytes {
		return fmt.Errorf("%s exceeds wire-format limit", name)
	}
	return nil
}

func writeTrainingStoreString(writer io.Writer, value string) error {
	return writeTrainingStorePayload(writer, []byte(value))
}

func writeTrainingStorePayload(writer io.Writer, payload []byte) error {
	if len(payload) > trainingStoreMaxWireStringBytes {
		return fmt.Errorf("payload exceeds %d-byte wire-format limit", trainingStoreMaxWireStringBytes)
	}
	var length [2]byte
	binary.LittleEndian.PutUint16(length[:], uint16(len(payload)))
	if err := writeTrainingBytes(writer, length[:]); err != nil {
		return err
	}
	return writeTrainingBytes(writer, payload)
}

func writeTrainingBytes(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func readTrainingStoreFile(persistPath string, maxSamples int) ([]TrainingSample, error) {
	persistPath = strings.TrimSpace(persistPath)
	if persistPath == "" {
		return nil, errors.New("training store path is empty")
	}
	absTarget, err := filepath.Abs(filepath.Clean(persistPath))
	if err != nil {
		return nil, fmt.Errorf("resolve training store path: %w", err)
	}
	rootPath, targetName, err := resolveTrainingStoreTarget(filepath.Dir(absTarget), absTarget)
	if err != nil {
		return nil, err
	}
	root, err := openTrainingStoreRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open training store directory: %w", err)
	}
	defer root.Close()

	file, err := platform.OpenBeneath(root, targetName, unix.O_RDONLY, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open training store file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat training store file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("training store must be a regular file")
	}
	if info.Size() < 0 || info.Size() > trainingStoreMaxFileBytes {
		return nil, fmt.Errorf("%w: file size is %d bytes", errTrainingStoreTooLarge, info.Size())
	}

	limited := &io.LimitedReader{R: file, N: trainingStoreMaxFileBytes + 1}
	reader := bufio.NewReaderSize(limited, 64*1024)
	samples, err := decodeTrainingStore(reader, maxSamples)
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func decodeTrainingStore(reader *bufio.Reader, maxSamples int) ([]TrainingSample, error) {
	var first [4]byte
	if _, err := io.ReadFull(reader, first[:]); err != nil {
		return nil, fmt.Errorf("read training store header: %w", err)
	}
	versioned := string(first[:]) == trainingStoreMagic
	count := binary.LittleEndian.Uint32(first[:])
	if versioned {
		var encodedCount [4]byte
		if _, err := io.ReadFull(reader, encodedCount[:]); err != nil {
			return nil, fmt.Errorf("read training store sample count: %w", err)
		}
		count = binary.LittleEndian.Uint32(encodedCount[:])
	}
	if count > trainingStoreMaxSamples {
		return nil, fmt.Errorf("training store sample count %d exceeds limit %d", count, trainingStoreMaxSamples)
	}

	capacity := NormalizeTrainingStoreCapacity(maxSamples)
	retained := make([]TrainingSample, 0, min(int(count), capacity))
	for index := uint32(0); index < count; index++ {
		sample, err := decodeTrainingSample(reader, versioned)
		if err != nil {
			return nil, fmt.Errorf("decode training sample %d: %w", index, err)
		}
		if len(retained) < capacity {
			if sample.Label >= 0 && sample.Label <= 3 {
				sample.UserLabel = "loaded"
			}
			retained = append(retained, NormalizeTrainingSample(sample))
		}
	}
	if _, err := reader.ReadByte(); err == nil {
		return nil, errors.New("training store contains trailing data")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("check training store trailer: %w", err)
	}
	return retained, nil
}

func decodeTrainingSample(reader io.Reader, versioned bool) (TrainingSample, error) {
	var sample TrainingSample
	var fixed [20]byte
	if _, err := io.ReadFull(reader, fixed[:]); err != nil {
		return sample, fmt.Errorf("read metadata: %w", err)
	}
	sample.Timestamp = unixNanoTime(int64(binary.LittleEndian.Uint64(fixed[0:8])))
	sample.Label = int32(binary.LittleEndian.Uint32(fixed[8:12]))
	sample.AnomalyScore = math.Float64frombits(binary.LittleEndian.Uint64(fixed[12:20]))

	if versioned {
		commandLine, err := readTrainingStoreString(reader)
		if err != nil {
			return sample, fmt.Errorf("read command line: %w", err)
		}
		sample.CommandLine = commandLine
	}
	comm, err := readTrainingStoreString(reader)
	if err != nil {
		return sample, fmt.Errorf("read command: %w", err)
	}
	sample.Comm = comm
	argsPayload, err := readTrainingStorePayload(reader)
	if err != nil {
		return sample, fmt.Errorf("read arguments: %w", err)
	}
	sample.Args, err = decodeTrainingStoreArgs(argsPayload)
	if err != nil {
		return sample, err
	}

	var features [FeatureDim * 8]byte
	if _, err := io.ReadFull(reader, features[:]); err != nil {
		return sample, fmt.Errorf("read features: %w", err)
	}
	for index := range sample.Features {
		sample.Features[index] = math.Float64frombits(binary.LittleEndian.Uint64(features[index*8 : (index+1)*8]))
	}
	return sample, nil
}

func unixNanoTime(nanoseconds int64) time.Time {
	return time.Unix(0, nanoseconds).UTC()
}

func readTrainingStoreString(reader io.Reader) (string, error) {
	payload, err := readTrainingStorePayload(reader)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func readTrainingStorePayload(reader io.Reader) ([]byte, error) {
	var encodedLength [2]byte
	if _, err := io.ReadFull(reader, encodedLength[:]); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint16(encodedLength[:]))
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeTrainingStoreArgs(payload []byte) ([]string, error) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err == nil {
		return args, nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		fallback := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
		if fallback == "" {
			return nil, nil
		}
		return behavior.SplitCommandLine(fallback), nil
	}
	return nil, errors.New("decode arguments: invalid JSON or legacy bracketed value")
}

func resolveTrainingStoreTarget(dataDir, persistPath string) (rootPath, targetName string, err error) {
	dataDir = strings.TrimSpace(dataDir)
	persistPath = strings.TrimSpace(persistPath)
	if dataDir == "" {
		return "", "", errors.New("training store data directory is empty")
	}
	if persistPath == "" {
		return "", "", errors.New("training store path is empty")
	}
	rootPath, err = filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return "", "", fmt.Errorf("resolve training store data directory: %w", err)
	}
	if rootPath == string(os.PathSeparator) {
		return "", "", errors.New("training store data directory must not be the filesystem root")
	}
	if !filepath.IsAbs(persistPath) {
		cleanTarget := filepath.Clean(persistPath)
		if filepath.Dir(cleanTarget) != "." {
			return "", "", errTrainingStorePathOutsideRoot
		}
		persistPath = filepath.Join(rootPath, cleanTarget)
	}
	absTarget, err := filepath.Abs(filepath.Clean(persistPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve training store path: %w", err)
	}
	if filepath.Dir(absTarget) != rootPath {
		return "", "", errTrainingStorePathOutsideRoot
	}
	targetName = filepath.Base(absTarget)
	if targetName == "." || targetName == ".." || strings.ContainsRune(targetName, 0) || len(targetName) > 240 {
		return "", "", errTrainingStorePathOutsideRoot
	}
	return rootPath, targetName, nil
}

func openTrainingStoreRoot(rootPath string) (*os.File, error) {
	root, err := platform.SecureOpenOrCreateDir(rootPath)
	if err != nil {
		return nil, err
	}
	if err := root.Chmod(0o700); err != nil {
		_ = root.Close()
		return nil, err
	}
	if err := platform.ChownArtifactFile(root); err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}
