# See https://developer.hashicorp.com/nomad/docs/secure/acl/policies for ACL Policy details

namespace "default" {
  capabilities = [
    # Allows GET /jobs and GET /jobs?filter=<prefix>
    "list-jobs",

    # Allows GET /jobs/{jobID}, GET /jobs/{jobID}/spec, GET /jobs/{jobID}/evaluations,
    # GET /jobs/{jobID}/allocations, GET /jobs/{jobID}/allocations/{allocID},
    # and GET /jobs/{jobID}/versions
    "read-job",

    # Allows POST /jobs (submit/update) and DELETE /jobs/{jobID} (stop/deregister)
    # and POST /jobs/{jobID}/revert
    "submit-job",

    # Allows GET /jobs/{jobID}/allocations/{allocID}/logs
    "read-logs",

    # Allows POST /jobs/{jobID}/allocations/{allocID}/restart
    "alloc-lifecycle",
  ]
}

# Needed for GET /health (verifies Nomad connectivity via agent self endpoint)
agent { policy = "read" }

# Needed for GET /node-pools and GET /node-pools/{poolName}/nodes
# (lets the MCP server reason about available capacity before scheduling jobs)
node { policy = "read" }
