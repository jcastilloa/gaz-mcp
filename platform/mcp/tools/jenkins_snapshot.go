// Package tools contains MCP tool definitions and handlers for Jenkins snapshot operations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	jenkinsApp "github.com/jcastillo/gaz-mcp/mcp/application/jenkins"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
	"github.com/mark3labs/mcp-go/mcp"
)

// JenkinsSnapshot groups all snapshot/versioning Jenkins MCP tools.
type JenkinsSnapshot struct {
	services map[string]*jenkinsApp.Service
}

// NewJenkinsSnapshot creates a JenkinsSnapshot tool group.
func NewJenkinsSnapshot(services map[string]*jenkinsApp.Service) JenkinsSnapshot {
	return JenkinsSnapshot{services: services}
}

func (j JenkinsSnapshot) envList() string {
	return strings.Join(jenkinsApp.SortedEnvNames(j.services), ", ")
}

func (j JenkinsSnapshot) resolveService(env string) (*jenkinsApp.Service, error) {
	svc, ok := j.services[env]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q, available: %s", env, j.envList())
	}
	return svc, nil
}

const snapshotTypeDesc = `Object type: "job", "view", "node", "credential", "folder"`

// --- jenkins_snapshot_list ---

func (j JenkinsSnapshot) ListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_snapshot_list",
		mcp.WithDescription(fmt.Sprintf(
			"List stored snapshots for a Jenkins object (jobs, views, nodes, credentials). Returns metadata sorted by version descending. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description(snapshotTypeDesc),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name (e.g. job name, view name)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default: 20)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Pagination offset (default: 0)"),
		),
	)
}

func (j JenkinsSnapshot) ListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	objType, objName, err := parseSnapshotTarget(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	limit := mcp.ParseInt(request, "limit", 20)
	offset := mcp.ParseInt(request, "offset", 0)
	if limit <= 0 {
		limit = 20
	}

	snapshots, err := svc.SnapshotList(ctx, env, objType, objName, limit, offset)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list snapshots: %v", err)), nil
	}

	out, _ := json.MarshalIndent(snapshots, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- jenkins_snapshot_get ---

func (j JenkinsSnapshot) GetDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_snapshot_get",
		mcp.WithDescription(fmt.Sprintf(
			"Get the full XML configuration stored in a specific snapshot version. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description(snapshotTypeDesc),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithNumber("version",
			mcp.Required(),
			mcp.Description("Snapshot version number (from jenkins_snapshot_list)"),
		),
	)
}

func (j JenkinsSnapshot) GetHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	objType, objName, err := parseSnapshotTarget(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	version := mcp.ParseInt(request, "version", 0)
	if version <= 0 {
		return mcp.NewToolResultError("version must be a positive integer"), nil
	}

	configXML, err := svc.SnapshotGet(ctx, env, objType, objName, version)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get snapshot: %v", err)), nil
	}
	return mcp.NewToolResultText(configXML), nil
}

// --- jenkins_snapshot_diff ---

func (j JenkinsSnapshot) DiffDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_snapshot_diff",
		mcp.WithDescription(fmt.Sprintf(
			"Compare two snapshot versions of a Jenkins object. Returns both XML configs for comparison. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description(snapshotTypeDesc),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithNumber("version_a",
			mcp.Required(),
			mcp.Description("First version number"),
		),
		mcp.WithNumber("version_b",
			mcp.Required(),
			mcp.Description("Second version number"),
		),
	)
}

func (j JenkinsSnapshot) DiffHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	objType, objName, err := parseSnapshotTarget(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	versionA := mcp.ParseInt(request, "version_a", 0)
	versionB := mcp.ParseInt(request, "version_b", 0)
	if versionA <= 0 || versionB <= 0 {
		return mcp.NewToolResultError("version_a and version_b must be positive integers"), nil
	}

	cfgA, cfgB, err := svc.SnapshotDiff(ctx, env, objType, objName, versionA, versionB)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("diff snapshots: %v", err)), nil
	}

	result := map[string]any{
		"object_type": string(objType),
		"object_name": objName,
		"version_a":   versionA,
		"version_b":   versionB,
		"config_a":    cfgA,
		"config_b":    cfgB,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- jenkins_snapshot_restore ---

func (j JenkinsSnapshot) RestoreDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_snapshot_restore",
		mcp.WithDescription(fmt.Sprintf(
			"Restore a Jenkins object to a previous snapshot version. A safety snapshot of the current state is taken before restoring. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description(snapshotTypeDesc),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithNumber("version",
			mcp.Required(),
			mcp.Description("Snapshot version to restore"),
		),
	)
}

func (j JenkinsSnapshot) RestoreHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	objType, objName, err := parseSnapshotTarget(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	version := mcp.ParseInt(request, "version", 0)
	if version <= 0 {
		return mcp.NewToolResultError("version must be a positive integer"), nil
	}

	if err := svc.SnapshotRestore(ctx, env, objType, objName, version); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("restore snapshot: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"%s %q restored to v%d. Current state was saved as a safety snapshot before restore.",
		objType, objName, version,
	)), nil
}

// --- jenkins_snapshot_prune ---

func (j JenkinsSnapshot) PruneDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_snapshot_prune",
		mcp.WithDescription(fmt.Sprintf(
			"Remove old snapshots for a Jenkins object, keeping only the N most recent versions. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description(snapshotTypeDesc),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithNumber("keep",
			mcp.Required(),
			mcp.Description("Number of most recent versions to keep"),
		),
	)
}

func (j JenkinsSnapshot) PruneHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	objType, objName, err := parseSnapshotTarget(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	keep := mcp.ParseInt(request, "keep", 0)
	if keep <= 0 {
		return mcp.NewToolResultError("keep must be a positive integer"), nil
	}

	deleted, err := svc.SnapshotPrune(ctx, env, objType, objName, keep)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("prune snapshots: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"Pruned %d old snapshot(s) for %s %q (keeping %d most recent).",
		deleted, objType, objName, keep,
	)), nil
}

// --- helpers ---

// parseSnapshotTarget extracts and validates object_type and object_name from a request.
func parseSnapshotTarget(request mcp.CallToolRequest) (domain.SnapshotType, string, error) {
	objTypeStr := strings.TrimSpace(mcp.ParseString(request, "object_type", ""))
	if objTypeStr == "" {
		return "", "", fmt.Errorf("object_type parameter is required")
	}

	objType := domain.SnapshotType(objTypeStr)
	switch objType {
	case domain.SnapshotJob, domain.SnapshotView, domain.SnapshotNode, domain.SnapshotCredential, domain.SnapshotFolder:
		// valid
	default:
		return "", "", fmt.Errorf("invalid object_type %q, must be one of: job, view, node, credential, folder", objTypeStr)
	}

	objName := strings.TrimSpace(mcp.ParseString(request, "object_name", ""))
	if objName == "" {
		return "", "", fmt.Errorf("object_name parameter is required")
	}

	return objType, objName, nil
}
