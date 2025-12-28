package event

import (
	"go.uber.org/zap"
)

// LoggingHandler logs all events
type LoggingHandler struct {
	logger *zap.Logger
}

// NewLoggingHandler creates a new LoggingHandler
func NewLoggingHandler(logger *zap.Logger) *LoggingHandler {
	return &LoggingHandler{logger: logger}
}

// Handle logs the event
func (h *LoggingHandler) Handle(event DomainEvent) error {
	switch e := event.(type) {
	case FileDownloaded:
		h.logger.Info("file downloaded",
			zap.Int64("file_id", e.FileID),
			zap.String("syno_path", e.SynoPath),
			zap.String("cache_path", e.CachePath),
			zap.Int64("size", e.Size),
			zap.Bool("resumed", e.Resumed),
		)
	case FileCacheInvalidated:
		h.logger.Info("file cache invalidated",
			zap.Int64("file_id", e.FileID),
			zap.String("syno_path", e.SynoPath),
			zap.String("reason", e.Reason),
		)
	case FileEvicted:
		h.logger.Info("file evicted",
			zap.Int64("file_id", e.FileID),
			zap.String("cache_path", e.CachePath),
			zap.Int64("size", e.Size),
			zap.Int("priority", e.Priority),
		)
	case ShareAccessed:
		h.logger.Debug("share accessed",
			zap.Int64("share_id", e.ShareID),
			zap.Int64("file_id", e.FileID),
			zap.String("client_ip", e.ClientIP),
		)
	case DownloadTaskCreated:
		h.logger.Debug("download task created",
			zap.Int64("task_id", e.TaskID),
			zap.Int64("file_id", e.FileID),
			zap.String("syno_path", e.SynoPath),
			zap.Int64("size", e.Size),
			zap.Int("priority", e.Priority),
		)
	case DownloadTaskFailed:
		h.logger.Warn("download task failed",
			zap.Int64("task_id", e.TaskID),
			zap.Int64("file_id", e.FileID),
			zap.String("syno_path", e.SynoPath),
			zap.String("error", e.Error),
			zap.Int("retry_count", e.RetryCount),
			zap.Bool("can_retry", e.CanRetry),
		)
	case DownloadTaskCompleted:
		h.logger.Info("download task completed",
			zap.Int64("task_id", e.TaskID),
			zap.Int64("file_id", e.FileID),
			zap.String("syno_path", e.SynoPath),
			zap.Int64("size", e.Size),
			zap.Duration("duration", e.Duration),
		)
	case SyncCompleted:
		h.logger.Info("sync completed",
			zap.String("sync_type", e.SyncType),
			zap.Int("files_added", e.FilesAdded),
			zap.Duration("duration", e.Duration),
		)
	default:
		h.logger.Debug("domain event",
			zap.String("event", event.EventName()),
			zap.Time("occurred_at", event.OccurredAt()),
		)
	}
	return nil
}

// HandledEvents returns the events this handler handles
func (h *LoggingHandler) HandledEvents() []string {
	return []string{"*"} // Handle all events
}

// MetricsHandler collects metrics from events
type MetricsHandler struct {
	// Counters for different events
	filesDownloaded   int64
	filesEvicted      int64
	downloadsFailed   int64
	sharesAccessed    int64
	bytesDownloaded   int64
	bytesEvicted      int64
}

// NewMetricsHandler creates a new MetricsHandler
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

// Handle updates metrics based on the event
func (h *MetricsHandler) Handle(event DomainEvent) error {
	switch e := event.(type) {
	case FileDownloaded:
		h.filesDownloaded++
		h.bytesDownloaded += e.Size
	case FileEvicted:
		h.filesEvicted++
		h.bytesEvicted += e.Size
	case DownloadTaskFailed:
		h.downloadsFailed++
	case ShareAccessed:
		h.sharesAccessed++
	}
	return nil
}

// HandledEvents returns the events this handler handles
func (h *MetricsHandler) HandledEvents() []string {
	return []string{
		"file.downloaded",
		"file.evicted",
		"download_task.failed",
		"share.accessed",
	}
}

// GetMetrics returns current metrics
func (h *MetricsHandler) GetMetrics() map[string]int64 {
	return map[string]int64{
		"files_downloaded":  h.filesDownloaded,
		"files_evicted":     h.filesEvicted,
		"downloads_failed":  h.downloadsFailed,
		"shares_accessed":   h.sharesAccessed,
		"bytes_downloaded":  h.bytesDownloaded,
		"bytes_evicted":     h.bytesEvicted,
	}
}
