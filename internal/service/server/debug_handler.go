package server

import (
	"encoding/json"
	"net/http"

	"github.com/vertextoedge/synology-file-cache/internal/port"
	"go.uber.org/zap"
)

// DebugHandler handles debug endpoint requests
type DebugHandler struct {
	store  port.Store
	logger *zap.Logger
}

// NewDebugHandler creates a new DebugHandler
func NewDebugHandler(store port.Store, logger *zap.Logger) *DebugHandler {
	return &DebugHandler{
		store:  store,
		logger: logger,
	}
}

// HandleStats handles debug statistics requests
func (h *DebugHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := h.store.GetCacheStats()
	if err != nil {
		h.logger.Error("failed to get cache stats", zap.Error(err))
		http.Error(w, "Failed to get cache stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleFiles handles debug file listing requests
func (h *DebugHandler) HandleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get cache stats
	stats, err := h.store.GetCacheStats()
	if err != nil {
		h.logger.Error("failed to get cache stats", zap.Error(err))
		http.Error(w, "Failed to get cache stats", http.StatusInternalServerError)
		return
	}

	// Get queue stats
	queueStats, err := h.store.GetQueueStats()
	if err != nil {
		h.logger.Error("failed to get queue stats", zap.Error(err))
		http.Error(w, "Failed to get queue stats", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"stats":       stats,
		"queue_stats": queueStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
