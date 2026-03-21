package nomad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/hashicorp/nomad/api"
)

const defaultHealthPollInterval = 5 * time.Second

// Client wraps the Nomad API client with helper methods for job and allocation management.
type Client struct {
	nomad              *api.Client
	log                *slog.Logger
	healthPollInterval time.Duration
}

// NewClient creates a new Client configured to talk to the Nomad agent at addr using the given ACL token.
func NewClient(addr, token string, log *slog.Logger) (*Client, error) {
	cfg := api.DefaultConfig()
	cfg.Address = addr
	cfg.SecretID = token

	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating nomad client: %w", err)
	}

	return &Client{nomad: c, log: log, healthPollInterval: defaultHealthPollInterval}, nil
}

// Ping verifies connectivity to Nomad by hitting the agent self endpoint.
func (c *Client) Ping() error {
	_, err := c.nomad.Agent().Self()
	if err != nil {
		return fmt.Errorf("nomad ping failed: %w", err)
	}
	return nil
}

// ListJobs returns jobs whose ID starts with prefix. An empty prefix returns all jobs.
func (c *Client) ListJobs(prefix string) ([]*api.JobListStub, error) {
	jobs, _, err := c.nomad.Jobs().List(&api.QueryOptions{Prefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	return jobs, nil
}

// GetJob returns the full job spec for the given job ID.
func (c *Client) GetJob(jobID string) (*api.Job, error) {
	job, _, err := c.nomad.Jobs().Info(jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting job %q: %w", jobID, err)
	}
	return job, nil
}

// GetJobSubmission returns the original source (HCL or JSON) that was submitted
// for the current version of the job. Requires Nomad 1.6+.
func (c *Client) GetJobSubmission(jobID string) (*api.JobSubmission, error) {
	job, _, err := c.nomad.Jobs().Info(jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting job %q: %w", jobID, err)
	}

	version := uint64(0)
	if job.Version != nil {
		version = *job.Version
	}

	sub, _, err := c.nomad.Jobs().Submission(jobID, int(version), nil)
	if err != nil {
		return nil, fmt.Errorf("getting submission for job %q version %d: %w", jobID, version, err)
	}
	return sub, nil
}

// SubmitJob parses a raw HCL job spec and registers it with Nomad,
// preserving the original HCL source so it remains accessible via the Nomad UI
// and the /jobs/{jobID}/spec endpoint.
func (c *Client) SubmitJob(hclSpec string) (*api.JobRegisterResponse, error) {
	job, err := c.nomad.Jobs().ParseHCL(hclSpec, true)
	if err != nil {
		return nil, fmt.Errorf("parsing job HCL: %w", err)
	}

	opts := &api.RegisterOptions{
		Submission: &api.JobSubmission{
			Source: hclSpec,
			Format: "hcl2",
		},
	}
	resp, _, err := c.nomad.Jobs().RegisterOpts(job, opts, nil)
	if err != nil {
		return nil, fmt.Errorf("registering job: %w", err)
	}
	return resp, nil
}

// GetAllocInfo returns full details for a single allocation, including
// allocated ports and task states.
func (c *Client) GetAllocInfo(allocID string) (*api.Allocation, error) {
	alloc, _, err := c.nomad.Allocations().Info(allocID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting allocation %q: %w", allocID, err)
	}
	return alloc, nil
}

// RestartAlloc restarts a task within an allocation in-place. If taskName is
// empty, all tasks in the allocation are restarted.
func (c *Client) RestartAlloc(allocID, taskName string) error {
	alloc, _, err := c.nomad.Allocations().Info(allocID, nil)
	if err != nil {
		return fmt.Errorf("getting allocation %q: %w", allocID, err)
	}
	if err := c.nomad.Allocations().Restart(alloc, taskName, nil); err != nil {
		return fmt.Errorf("restarting allocation %q: %w", allocID, err)
	}
	return nil
}

// GetJobVersions returns the full version history for a job.
func (c *Client) GetJobVersions(jobID string) ([]*api.Job, error) {
	versions, _, _, err := c.nomad.Jobs().Versions(jobID, false, nil)
	if err != nil {
		return nil, fmt.Errorf("getting versions for job %q: %w", jobID, err)
	}
	return versions, nil
}

// RevertJob reverts a job to the specified version.
func (c *Client) RevertJob(jobID string, version uint64) (*api.JobRegisterResponse, error) {
	resp, _, err := c.nomad.Jobs().Revert(jobID, version, nil, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("reverting job %q to version %d: %w", jobID, version, err)
	}
	return resp, nil
}

// ListNodePools returns all node pools.
func (c *Client) ListNodePools() ([]*api.NodePool, error) {
	pools, _, err := c.nomad.NodePools().List(nil)
	if err != nil {
		return nil, fmt.Errorf("listing node pools: %w", err)
	}
	return pools, nil
}

// ListNodesInPool returns all nodes belonging to the given node pool,
// including their total and available resources.
func (c *Client) ListNodesInPool(poolName string) ([]*api.NodeListStub, error) {
	nodes, _, err := c.nomad.Nodes().List(&api.QueryOptions{
		Params: map[string]string{"node_pool": poolName},
	})
	if err != nil {
		return nil, fmt.Errorf("listing nodes in pool %q: %w", poolName, err)
	}
	return nodes, nil
}

// GetEvaluations returns all evaluations for the given job, most recent first.
func (c *Client) GetEvaluations(jobID string) ([]*api.Evaluation, error) {
	evals, _, err := c.nomad.Jobs().Evaluations(jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting evaluations for job %q: %w", jobID, err)
	}
	return evals, nil
}

// GetAllocations returns all allocations for the current version of the given job.
func (c *Client) GetAllocations(jobID string) ([]*api.AllocationListStub, error) {
	allocs, _, err := c.nomad.Jobs().Allocations(jobID, false, nil)
	if err != nil {
		return nil, fmt.Errorf("getting allocations for job %q: %w", jobID, err)
	}
	return allocs, nil
}

// DefaultLogLimitBytes is the default byte limit for log retrieval (50KB).
const DefaultLogLimitBytes = 50 * 1024

// GetAllocLogs returns task logs for the given allocation.
// logType is "stdout" or "stderr". origin is "start" or "end".
// limitBytes controls how many bytes to return; 0 means no limit.
func (c *Client) GetAllocLogs(allocID, task, logType, origin string, limitBytes int64) (string, error) {
	alloc, _, err := c.nomad.Allocations().Info(allocID, nil)
	if err != nil {
		return "", fmt.Errorf("getting allocation %q: %w", allocID, err)
	}

	// offset is how many bytes from the origin to start reading.
	// For origin=end this gives us the last limitBytes of the log.
	// For origin=start with limitBytes=0 we read from the very beginning.
	var offset int64
	if limitBytes > 0 && origin == "end" {
		offset = limitBytes
	}

	cancel := make(chan struct{})
	defer close(cancel)

	frames, errCh := c.nomad.AllocFS().Logs(alloc, false, task, logType, origin, offset, cancel, nil)

	var buf []byte
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				// frames channel closed — drain errCh for any real error.
				select {
				case err := <-errCh:
					// io.EOF is normal end-of-stream for a non-follow log read.
					if err != nil && !errors.Is(err, io.EOF) {
						return "", fmt.Errorf("reading logs for alloc %q: %w", allocID, err)
					}
				default:
				}
				return string(buf), nil
			}
			if frame != nil {
				buf = append(buf, frame.Data...)
			}
		case err := <-errCh:
			// io.EOF means the log stream ended normally.
			if err != nil && !errors.Is(err, io.EOF) {
				return "", fmt.Errorf("reading logs for alloc %q: %w", allocID, err)
			}
			// Drain remaining frames then return.
			for frame := range frames {
				if frame != nil {
					buf = append(buf, frame.Data...)
				}
			}
			return string(buf), nil
		}
	}
}

// StopJobResponse holds the result of a stop/deregister operation.
type StopJobResponse struct {
	EvalID string `json:"eval_id"`
}

// StopJob deregisters a job. If purge is true, the job is fully removed from Nomad.
func (c *Client) StopJob(jobID string, purge bool) (*StopJobResponse, error) {
	evalID, _, err := c.nomad.Jobs().Deregister(jobID, purge, nil)
	if err != nil {
		return nil, fmt.Errorf("stopping job %q: %w", jobID, err)
	}
	return &StopJobResponse{EvalID: evalID}, nil
}

// WatchJobHealth polls allocations for jobID until all are running and healthy,
// or until ctx is cancelled (e.g. deadline exceeded).
// Returns true if all allocations became healthy, false if ctx expired first.
func (c *Client) WatchJobHealth(ctx context.Context, jobID string) (bool, error) {
	ticker := time.NewTicker(c.healthPollInterval)
	defer ticker.Stop()

	for {
		healthy, err := c.checkJobHealth(jobID)
		if err != nil {
			return false, err
		}
		if healthy {
			return true, nil
		}

		select {
		case <-ctx.Done():
			return false, nil
		case <-ticker.C:
		}
	}
}

// checkJobHealth returns true when the job has at least one allocation and all
// allocations are running and healthy.
func (c *Client) checkJobHealth(jobID string) (bool, error) {
	allocs, _, err := c.nomad.Jobs().Allocations(jobID, false, nil)
	if err != nil {
		return false, fmt.Errorf("fetching allocations for job %q: %w", jobID, err)
	}
	if len(allocs) == 0 {
		c.log.Debug("no allocations yet", "job_id", jobID)
		return false, nil
	}

	for _, a := range allocs {
		// Skip terminal allocations (failed, lost, complete) — only consider running ones
		if a.ClientStatus == "complete" || a.ClientStatus == "failed" || a.ClientStatus == "lost" {
			return false, fmt.Errorf("job %q has a terminal allocation (status: %s)", jobID, a.ClientStatus)
		}
		if a.ClientStatus != "running" {
			c.log.Debug("allocation not yet running", "job_id", jobID, "alloc_id", a.ID, "status", a.ClientStatus)
			return false, nil
		}
		// Require deployment health to be explicitly confirmed.
		// DeploymentStatus is nil until Nomad's deployment tracker assesses the allocation,
		// and Healthy is nil while the assessment is in progress — treat both as "not yet ready".
		if a.DeploymentStatus == nil || a.DeploymentStatus.Healthy == nil || !*a.DeploymentStatus.Healthy {
			c.log.Debug("allocation deployment health not yet confirmed",
				"job_id", jobID, "alloc_id", a.ID,
				"has_deployment_status", a.DeploymentStatus != nil)
			return false, nil
		}
	}

	return true, nil
}
