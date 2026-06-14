// Package tools contains MCP tool definitions and handlers for Jenkins read operations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	jenkinsApp "github.com/jcastillo/gaz-mcp/mcp/application/jenkins"
	"github.com/mark3labs/mcp-go/mcp"
)

// JenkinsRead groups all read-only Jenkins MCP tools.
type JenkinsRead struct {
	services map[string]*jenkinsApp.Service
}

// NewJenkinsRead creates a JenkinsRead tool group.
func NewJenkinsRead(services map[string]*jenkinsApp.Service) JenkinsRead {
	return JenkinsRead{services: services}
}

// Services returns the underlying service map (used for DI wiring checks).
func (j JenkinsRead) Services() map[string]*jenkinsApp.Service {
	return j.services
}

// envList returns a sorted comma-separated list of available environment names.
func (j JenkinsRead) envList() string {
	return strings.Join(jenkinsApp.SortedEnvNames(j.services), ", ")
}

func (j JenkinsRead) resolveService(env string) (*jenkinsApp.Service, error) {
	svc, ok := j.services[env]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q, available: %s", env, j.envList())
	}
	return svc, nil
}

func parseEnv(request mcp.CallToolRequest) string {
	return strings.TrimSpace(mcp.ParseString(request, "environment", ""))
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// --- jenkins_info ---

func (j JenkinsRead) InfoDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_info",
		mcp.WithDescription(fmt.Sprintf(
			"Get general information about a Jenkins instance (version, job count, node count, quiet mode). Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
	)
}

func (j JenkinsRead) InfoHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	info, err := svc.Info(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("jenkins info: %v", err)), nil
	}
	return jsonResult(info)
}

// --- jenkins_job_list ---

func (j JenkinsRead) JobListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_list",
		mcp.WithDescription(fmt.Sprintf(
			"List Jenkins jobs. Optionally filter by name substring. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("filter",
			mcp.Description("Optional substring to filter job names (case-insensitive)"),
		),
	)
}

func (j JenkinsRead) JobListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	filter := strings.TrimSpace(mcp.ParseString(request, "filter", ""))
	jobs, err := svc.JobList(ctx, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list jobs: %v", err)), nil
	}
	return jsonResult(jobs)
}

// --- jenkins_job_get ---

func (j JenkinsRead) JobGetDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_get",
		mcp.WithDescription(fmt.Sprintf(
			"Get detailed information about a specific Jenkins job. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name (use full path for nested jobs, e.g. 'folder/job-name')"),
		),
	)
}

func (j JenkinsRead) JobGetHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	job := strings.TrimSpace(mcp.ParseString(request, "job", ""))
	if job == "" {
		return mcp.NewToolResultError("job parameter is required"), nil
	}
	info, err := svc.JobGet(ctx, job)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get job: %v", err)), nil
	}
	return jsonResult(info)
}

// --- jenkins_job_config ---

func (j JenkinsRead) JobConfigDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_config",
		mcp.WithDescription(fmt.Sprintf(
			"Get the XML configuration of a Jenkins job. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name"),
		),
	)
}

func (j JenkinsRead) JobConfigHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	job := strings.TrimSpace(mcp.ParseString(request, "job", ""))
	if job == "" {
		return mcp.NewToolResultError("job parameter is required"), nil
	}
	cfg, err := svc.JobConfig(ctx, job)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get job config: %v", err)), nil
	}
	return mcp.NewToolResultText(cfg), nil
}

// --- jenkins_build_info ---

func (j JenkinsRead) BuildInfoDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_build_info",
		mcp.WithDescription(fmt.Sprintf(
			"Get information about a specific Jenkins build (result, duration, causes, parameters). Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name"),
		),
		mcp.WithNumber("build_number",
			mcp.Required(),
			mcp.Description("Build number"),
		),
	)
}

func (j JenkinsRead) BuildInfoHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	job := strings.TrimSpace(mcp.ParseString(request, "job", ""))
	if job == "" {
		return mcp.NewToolResultError("job parameter is required"), nil
	}
	buildNum := mcp.ParseInt(request, "build_number", 0)
	if buildNum <= 0 {
		return mcp.NewToolResultError("build_number must be a positive integer"), nil
	}
	info, err := svc.BuildInfo(ctx, job, buildNum)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get build info: %v", err)), nil
	}
	return jsonResult(info)
}

// --- jenkins_build_log ---

func (j JenkinsRead) BuildLogDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_build_log",
		mcp.WithDescription(fmt.Sprintf(
			"Get the console log of a Jenkins build. Supports pagination via start_line. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name"),
		),
		mcp.WithNumber("build_number",
			mcp.Required(),
			mcp.Description("Build number"),
		),
		mcp.WithNumber("start_line",
			mcp.Description("Line offset to start reading from (default: 0)"),
		),
	)
}

func (j JenkinsRead) BuildLogHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	job := strings.TrimSpace(mcp.ParseString(request, "job", ""))
	if job == "" {
		return mcp.NewToolResultError("job parameter is required"), nil
	}
	buildNum := mcp.ParseInt(request, "build_number", 0)
	if buildNum <= 0 {
		return mcp.NewToolResultError("build_number must be a positive integer"), nil
	}
	startLine := mcp.ParseInt(request, "start_line", 0)

	log, totalLines, err := svc.BuildLog(ctx, job, buildNum, startLine)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get build log: %v", err)), nil
	}

	result := map[string]any{
		"log":         log,
		"total_lines": totalLines,
		"start_line":  startLine,
	}
	return jsonResult(result)
}

// --- jenkins_build_artifacts ---

func (j JenkinsRead) BuildArtifactsDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_build_artifacts",
		mcp.WithDescription(fmt.Sprintf(
			"List artifacts produced by a Jenkins build. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name"),
		),
		mcp.WithNumber("build_number",
			mcp.Required(),
			mcp.Description("Build number"),
		),
	)
}

func (j JenkinsRead) BuildArtifactsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	job := strings.TrimSpace(mcp.ParseString(request, "job", ""))
	if job == "" {
		return mcp.NewToolResultError("job parameter is required"), nil
	}
	buildNum := mcp.ParseInt(request, "build_number", 0)
	if buildNum <= 0 {
		return mcp.NewToolResultError("build_number must be a positive integer"), nil
	}
	artifacts, err := svc.BuildArtifacts(ctx, job, buildNum)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get artifacts: %v", err)), nil
	}
	return jsonResult(artifacts)
}

// --- jenkins_node_list ---

func (j JenkinsRead) NodeListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_node_list",
		mcp.WithDescription(fmt.Sprintf(
			"List all Jenkins nodes/agents with their online/idle status. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
	)
}

func (j JenkinsRead) NodeListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	nodes, err := svc.NodeList(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list nodes: %v", err)), nil
	}
	return jsonResult(nodes)
}

// --- jenkins_queue_list ---

func (j JenkinsRead) QueueListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_queue_list",
		mcp.WithDescription(fmt.Sprintf(
			"List items currently in the Jenkins build queue. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
	)
}

func (j JenkinsRead) QueueListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	items, err := svc.QueueList(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list queue: %v", err)), nil
	}
	return jsonResult(items)
}

// --- jenkins_plugin_list ---

func (j JenkinsRead) PluginListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_plugin_list",
		mcp.WithDescription(fmt.Sprintf(
			"List all installed Jenkins plugins with their version and enabled status. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
	)
}

func (j JenkinsRead) PluginListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	plugins, err := svc.PluginList(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list plugins: %v", err)), nil
	}
	return jsonResult(plugins)
}

// --- jenkins_view_list ---

func (j JenkinsRead) ViewListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_view_list",
		mcp.WithDescription(fmt.Sprintf(
			"List all Jenkins views. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
	)
}

func (j JenkinsRead) ViewListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	views, err := svc.ViewList(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list views: %v", err)), nil
	}
	return jsonResult(views)
}

// --- jenkins_credential_list ---

func (j JenkinsRead) CredentialListDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_credential_list",
		mcp.WithDescription(fmt.Sprintf(
			"List credentials in a Jenkins credentials store (without secret values). Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("store",
			mcp.Description("Credentials store name (default: 'system')"),
		),
		mcp.WithString("domain",
			mcp.Description("Credentials domain (default: '_' for global domain)"),
		),
	)
}

func (j JenkinsRead) CredentialListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	store := strings.TrimSpace(mcp.ParseString(request, "store", "system"))
	domain := strings.TrimSpace(mcp.ParseString(request, "domain", "_"))
	creds, err := svc.CredentialList(ctx, store, domain)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list credentials: %v", err)), nil
	}
	return jsonResult(creds)
}
