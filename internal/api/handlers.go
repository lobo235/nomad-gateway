package api

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/lobo235/nomad-gateway/internal/nomad"
)

const defaultHealthTimeout = 5 * time.Minute

// listJobsHandler handles GET /jobs?filter=<prefix>
func (s *Server) listJobsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("filter")
		jobs, err := s.nomad.ListJobs(prefix)
		if err != nil {
			s.log.Error("list jobs failed", "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

// getJobHandler handles GET /jobs/{jobID}
func (s *Server) getJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		job, err := s.nomad.GetJob(jobID)
		if err != nil {
			s.log.Error("get job failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

// getJobSpecHandler handles GET /jobs/{jobID}/spec
// Returns the original source (HCL or JSON) that was submitted for the current job version.
func (s *Server) getJobSpecHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		sub, err := s.nomad.GetJobSubmission(jobID)
		if err != nil {
			s.log.Error("get job submission failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sub)
	}
}

// submitJobHandler handles POST /jobs
// Request body: raw HCL job spec
func (s *Server) submitJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB limit
		if err != nil {
			writeError(w, http.StatusBadRequest, "read_error", "failed to read request body")
			return
		}
		if len(body) == 0 {
			writeError(w, http.StatusBadRequest, "empty_body", "request body must contain an HCL job spec")
			return
		}

		resp, err := s.nomad.SubmitJob(string(body))
		if err != nil {
			s.log.Error("submit job failed", "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// stopJobHandler handles DELETE /jobs/{jobID}?purge=<bool>
func (s *Server) stopJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")

		purge := false
		if p := r.URL.Query().Get("purge"); p != "" {
			var parseErr error
			purge, parseErr = strconv.ParseBool(p)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, "invalid_param", "purge must be true or false")
				return
			}
		}

		resp, err := s.nomad.StopJob(jobID, purge)
		if err != nil {
			s.log.Error("stop job failed", "job_id", jobID, "purge", purge, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// getAllocInfoHandler handles GET /jobs/{jobID}/allocations/{allocID}
func (s *Server) getAllocInfoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		allocID := r.PathValue("allocID")
		alloc, err := s.nomad.GetAllocInfo(allocID)
		if err != nil {
			s.log.Error("get alloc info failed", "job_id", jobID, "alloc_id", allocID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, alloc)
	}
}

// restartAllocHandler handles POST /jobs/{jobID}/allocations/{allocID}/restart
// Optional query param: task=<taskName> — restarts all tasks if omitted.
func (s *Server) restartAllocHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		allocID := r.PathValue("allocID")
		taskName := r.URL.Query().Get("task")

		if err := s.nomad.RestartAlloc(allocID, taskName); err != nil {
			s.log.Error("restart alloc failed", "job_id", jobID, "alloc_id", allocID, "task", taskName, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
	}
}

// getJobVersionsHandler handles GET /jobs/{jobID}/versions
func (s *Server) getJobVersionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		versions, err := s.nomad.GetJobVersions(jobID)
		if err != nil {
			s.log.Error("get job versions failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

// revertJobHandler handles POST /jobs/{jobID}/revert?version=<N>
func (s *Server) revertJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")

		vStr := r.URL.Query().Get("version")
		if vStr == "" {
			writeError(w, http.StatusBadRequest, "missing_param", "version query parameter is required")
			return
		}
		version, err := strconv.ParseUint(vStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "version must be a non-negative integer")
			return
		}

		resp, err := s.nomad.RevertJob(jobID, version)
		if err != nil {
			s.log.Error("revert job failed", "job_id", jobID, "version", version, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// listNodePoolsHandler handles GET /node-pools
func (s *Server) listNodePoolsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pools, err := s.nomad.ListNodePools()
		if err != nil {
			s.log.Error("list node pools failed", "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, pools)
	}
}

// listNodesInPoolHandler handles GET /node-pools/{poolName}/nodes
func (s *Server) listNodesInPoolHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolName := r.PathValue("poolName")
		nodes, err := s.nomad.ListNodesInPool(poolName)
		if err != nil {
			s.log.Error("list nodes in pool failed", "pool", poolName, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, nodes)
	}
}

// getEvaluationsHandler handles GET /jobs/{jobID}/evaluations
func (s *Server) getEvaluationsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		evals, err := s.nomad.GetEvaluations(jobID)
		if err != nil {
			s.log.Error("get evaluations failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, evals)
	}
}

// getAllocationsHandler handles GET /jobs/{jobID}/allocations
func (s *Server) getAllocationsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		allocs, err := s.nomad.GetAllocations(jobID)
		if err != nil {
			s.log.Error("get allocations failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, allocs)
	}
}

// getLogsHandler handles GET /jobs/{jobID}/allocations/{allocID}/logs
// Query params:
//   - task   (required) task name within the allocation
//   - type   stdout|stderr (default: stdout)
//   - origin start|end    (default: end)
//   - limit  bytes to return, 0 = unlimited (default: 51200)
func (s *Server) getLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")
		allocID := r.PathValue("allocID")

		task := r.URL.Query().Get("task")
		if task == "" {
			writeError(w, http.StatusBadRequest, "missing_param", "task query parameter is required")
			return
		}

		logType := r.URL.Query().Get("type")
		if logType == "" {
			logType = "stdout"
		}
		if logType != "stdout" && logType != "stderr" {
			writeError(w, http.StatusBadRequest, "invalid_param", "type must be stdout or stderr")
			return
		}

		origin := r.URL.Query().Get("origin")
		if origin == "" {
			origin = "end"
		}
		if origin != "start" && origin != "end" {
			writeError(w, http.StatusBadRequest, "invalid_param", "origin must be start or end")
			return
		}

		limitBytes := int64(nomad.DefaultLogLimitBytes)
		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 64)
			if err != nil || parsed < 0 {
				writeError(w, http.StatusBadRequest, "invalid_param", "limit must be a non-negative integer (bytes); 0 means all logs")
				return
			}
			limitBytes = parsed
		}

		// limit=0 means "all logs" — force origin=start so the offset calculation
		// in GetAllocLogs reads from the beginning rather than 0 bytes from the end.
		if limitBytes == 0 {
			origin = "start"
		}

		logs, err := s.nomad.GetAllocLogs(allocID, task, logType, origin, limitBytes)
		if err != nil {
			s.log.Error("get logs failed", "job_id", jobID, "alloc_id", allocID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(logs))
	}
}

type jobHealthResponse struct {
	JobID   string `json:"job_id"`
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
}

// watchJobHealthHandler handles GET /jobs/{jobID}/health?timeout=<duration>
// Blocks until all allocations are healthy or the timeout is reached.
// Returns 200 with {"healthy": true} on success, 408 with an error body on timeout.
func (s *Server) watchJobHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")

		timeout := defaultHealthTimeout
		if t := r.URL.Query().Get("timeout"); t != "" {
			var parseErr error
			timeout, parseErr = time.ParseDuration(t)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, "invalid_param",
					"timeout must be a Go duration string (e.g. 5m, 30s, 2m30s)")
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		healthy, err := s.nomad.WatchJobHealth(ctx, jobID)
		if err != nil {
			s.log.Error("watch job health failed", "job_id", jobID, "error", err)
			writeError(w, http.StatusBadGateway, "nomad_error", err.Error())
			return
		}

		if !healthy {
			writeError(w, http.StatusRequestTimeout, "timeout",
				"job did not become healthy within the timeout period")
			return
		}

		writeJSON(w, http.StatusOK, jobHealthResponse{JobID: jobID, Healthy: true, Status: "all allocations running and healthy"})
	}
}
