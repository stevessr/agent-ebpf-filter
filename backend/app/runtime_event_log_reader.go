package app

import (
	"context"
	"os"
)

func tailCapturedEventsFileAtRootContext(ctx context.Context, root, path string, limit int) ([]CapturedEventRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	} else if limit > runtimeEventLogMaxRecords {
		limit = runtimeEventLogMaxRecords
	}
	file, _, err := openRuntimeEventLogFileWithin(root, path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return readCapturedEventTail(ctx, file, info.Size(), limit)
}
