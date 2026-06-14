// Package tools contains MCP tool definitions and handlers for Jenkins write/execute operations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	jenkinsApp "github.com/jcastillo/gaz-mcp/mcp/application/jenkins"
	"github.com/mark3labs/mcp-go/mcp"
)

// JenkinsWrite groups all write/execute Jenkins MCP tools.
type JenkinsWrite struct {
	services map[string]*jenkinsApp.Service
}

// NewJenkinsWrite creates a JenkinsWrite tool group.
func NewJenkinsWrite(services map[string]*jenkinsApp.Service) JenkinsWrite {
	return JenkinsWrite{services: services}
}

func (j JenkinsWrite) envList() string {
	return strings.Join(jenkinsApp.SortedEnvNames(j.services), ", ")
}

func (j JenkinsWrite) resolveService(env string) (*jenkinsApp.Service, error) {
	svc, ok := j.services[env]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q, available: %s", env, j.envList())
	}
	return svc, nil
}

// --- jenkins_job_set_config ---

func (j JenkinsWrite) JobSetConfigDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_set_config",
		mcp.WithDescription(fmt.Sprintf(
			"Update the XML configuration of a Jenkins job. Automatically snapshots the previous config before applying changes. Available environments: %s.",
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
		mcp.WithString("config_xml",
			mcp.Required(),
			mcp.Description("Full Jenkins job XML configuration"),
		),
	)
}

func (j JenkinsWrite) JobSetConfigHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	configXML := mcp.ParseString(request, "config_xml", "")
	if strings.TrimSpace(configXML) == "" {
		return mcp.NewToolResultError("config_xml parameter is required"), nil
	}
	version, err := svc.JobSetConfig(ctx, env, job, configXML)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("set job config: %v", err)), nil
	}
	if version == 0 {
		return mcp.NewToolResultText("No changes detected — config is identical to current version."), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q updated. Previous config saved as snapshot v%d.", job, version)), nil
}

// --- jenkins_job_create ---

func (j JenkinsWrite) JobCreateDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_create",
		mcp.WithDescription(fmt.Sprintf(
			"Create a new Jenkins job from an XML configuration. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("New job name"),
		),
		mcp.WithString("config_xml",
			mcp.Required(),
			mcp.Description("Jenkins job XML configuration"),
		),
	)
}

func (j JenkinsWrite) JobCreateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	configXML := mcp.ParseString(request, "config_xml", "")
	if strings.TrimSpace(configXML) == "" {
		return mcp.NewToolResultError("config_xml parameter is required"), nil
	}
	version, err := svc.JobCreate(ctx, env, job, configXML)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create job: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q created and snapshotted as v%d.", job, version)), nil
}

// --- jenkins_job_copy ---

func (j JenkinsWrite) JobCopyDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_copy",
		mcp.WithDescription(fmt.Sprintf(
			"Copy an existing Jenkins job to a new name. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("from",
			mcp.Required(),
			mcp.Description("Source job name"),
		),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("Destination job name"),
		),
	)
}

func (j JenkinsWrite) JobCopyHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	from := strings.TrimSpace(mcp.ParseString(request, "from", ""))
	to := strings.TrimSpace(mcp.ParseString(request, "to", ""))
	if from == "" || to == "" {
		return mcp.NewToolResultError("from and to parameters are required"), nil
	}
	version, err := svc.JobCopy(ctx, env, from, to)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy job: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q copied to %q (snapshot v%d).", from, to, version)), nil
}

// --- jenkins_job_delete ---

func (j JenkinsWrite) JobDeleteDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_delete",
		mcp.WithDescription(fmt.Sprintf(
			"Delete a Jenkins job. The config is automatically snapshotted before deletion for recovery. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("job",
			mcp.Required(),
			mcp.Description("Job name to delete"),
		),
	)
}

func (j JenkinsWrite) JobDeleteHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	version, err := svc.JobDelete(ctx, env, job)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete job: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q deleted. Config saved as snapshot v%d for recovery.", job, version)), nil
}

// --- jenkins_job_enable ---

func (j JenkinsWrite) JobEnableDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_enable",
		mcp.WithDescription(fmt.Sprintf(
			"Enable a disabled Jenkins job. Available environments: %s.",
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

func (j JenkinsWrite) JobEnableHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if err := svc.JobEnable(ctx, job); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("enable job: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q enabled.", job)), nil
}

// --- jenkins_job_disable ---

func (j JenkinsWrite) JobDisableDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_disable",
		mcp.WithDescription(fmt.Sprintf(
			"Disable a Jenkins job. Available environments: %s.",
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

func (j JenkinsWrite) JobDisableHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if err := svc.JobDisable(ctx, job); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("disable job: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q disabled.", job)), nil
}

// --- jenkins_job_build ---

func (j JenkinsWrite) JobBuildDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_job_build",
		mcp.WithDescription(fmt.Sprintf(
			"Trigger a Jenkins job build. Returns the queue item ID. Available environments: %s.",
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
		mcp.WithString("params",
			mcp.Description(`Optional build parameters as JSON object, e.g. {"BRANCH":"main","DEBUG":"true"}`),
		),
	)
}

func (j JenkinsWrite) JobBuildHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	var params map[string]string
	if raw := strings.TrimSpace(mcp.ParseString(request, "params", "")); raw != "" {
		if err := jsonUnmarshal(raw, &params); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid params JSON: %v", err)), nil
		}
	}

	queueID, err := svc.JobBuild(ctx, job, params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("trigger build: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Build triggered for job %q. Queue item ID: %d.", job, queueID)), nil
}

// --- jenkins_build_stop ---

func (j JenkinsWrite) BuildStopDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_build_stop",
		mcp.WithDescription(fmt.Sprintf(
			"Stop a running Jenkins build. Available environments: %s.",
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
			mcp.Description("Build number to stop"),
		),
	)
}

func (j JenkinsWrite) BuildStopHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if err := svc.BuildStop(ctx, job, buildNum); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stop build: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Build #%d of job %q stopped.", buildNum, job)), nil
}

// --- jenkins_queue_cancel ---

func (j JenkinsWrite) QueueCancelDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_queue_cancel",
		mcp.WithDescription(fmt.Sprintf(
			"Cancel a queued Jenkins build by queue item ID. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithNumber("queue_id",
			mcp.Required(),
			mcp.Description("Queue item ID (from jenkins_queue_list)"),
		),
	)
}

func (j JenkinsWrite) QueueCancelHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	queueID := mcp.ParseInt64(request, "queue_id", 0)
	if queueID <= 0 {
		return mcp.NewToolResultError("queue_id must be a positive integer"), nil
	}
	if err := svc.QueueCancel(ctx, queueID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cancel queue item: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Queue item %d cancelled.", queueID)), nil
}

// --- jenkins_node_enable ---

func (j JenkinsWrite) NodeEnableDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_node_enable",
		mcp.WithDescription(fmt.Sprintf(
			"Bring a Jenkins node/agent back online. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name"),
		),
	)
}

func (j JenkinsWrite) NodeEnableHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	node := strings.TrimSpace(mcp.ParseString(request, "node", ""))
	if node == "" {
		return mcp.NewToolResultError("node parameter is required"), nil
	}
	if err := svc.NodeEnable(ctx, node); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("enable node: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Node %q brought online.", node)), nil
}

// --- jenkins_node_disable ---

func (j JenkinsWrite) NodeDisableDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_node_disable",
		mcp.WithDescription(fmt.Sprintf(
			"Take a Jenkins node/agent offline. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name"),
		),
		mcp.WithString("message",
			mcp.Description("Reason for taking the node offline"),
		),
	)
}

func (j JenkinsWrite) NodeDisableHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	node := strings.TrimSpace(mcp.ParseString(request, "node", ""))
	if node == "" {
		return mcp.NewToolResultError("node parameter is required"), nil
	}
	message := mcp.ParseString(request, "message", "Taken offline by gaz-mcp")
	if err := svc.NodeDisable(ctx, node, message); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("disable node: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Node %q taken offline: %s", node, message)), nil
}

// --- jenkins_script_console ---

func (j JenkinsWrite) ScriptConsoleDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_script_console",
		mcp.WithDescription(fmt.Sprintf(
			"Execute a Groovy script on the Jenkins script console. Use with caution — this has full access to the Jenkins instance. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("script",
			mcp.Required(),
			mcp.Description("Groovy script to execute"),
		),
	)
}

func (j JenkinsWrite) ScriptConsoleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	script := strings.TrimSpace(mcp.ParseString(request, "script", ""))
	if script == "" {
		return mcp.NewToolResultError("script parameter is required"), nil
	}
	output, err := svc.ScriptConsole(ctx, script)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("script console: %v", err)), nil
	}
	return mcp.NewToolResultText(output), nil
}

// --- jenkins_credential_create ---

func (j JenkinsWrite) CredentialCreateDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_credential_create",
		mcp.WithDescription(fmt.Sprintf(
			"Create a Jenkins credential from XML configuration. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Credential ID"),
		),
		mcp.WithString("config_xml",
			mcp.Required(),
			mcp.Description("Jenkins credential XML configuration"),
		),
		mcp.WithString("store",
			mcp.Description("Credentials store (default: 'system')"),
		),
		mcp.WithString("domain",
			mcp.Description("Credentials domain (default: '_' for global)"),
		),
	)
}

func (j JenkinsWrite) CredentialCreateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id := strings.TrimSpace(mcp.ParseString(request, "id", ""))
	if id == "" {
		return mcp.NewToolResultError("id parameter is required"), nil
	}
	configXML := mcp.ParseString(request, "config_xml", "")
	if strings.TrimSpace(configXML) == "" {
		return mcp.NewToolResultError("config_xml parameter is required"), nil
	}
	store := strings.TrimSpace(mcp.ParseString(request, "store", "system"))
	domain := strings.TrimSpace(mcp.ParseString(request, "domain", "_"))

	version, err := svc.CredentialCreate(ctx, env, store, domain, id, configXML)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create credential: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Credential %q created (snapshot v%d).", id, version)), nil
}

// --- jenkins_credential_delete ---

func (j JenkinsWrite) CredentialDeleteDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_credential_delete",
		mcp.WithDescription(fmt.Sprintf(
			"Delete a Jenkins credential. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Credential ID to delete"),
		),
		mcp.WithString("store",
			mcp.Description("Credentials store (default: 'system')"),
		),
		mcp.WithString("domain",
			mcp.Description("Credentials domain (default: '_' for global)"),
		),
	)
}

func (j JenkinsWrite) CredentialDeleteHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id := strings.TrimSpace(mcp.ParseString(request, "id", ""))
	if id == "" {
		return mcp.NewToolResultError("id parameter is required"), nil
	}
	store := strings.TrimSpace(mcp.ParseString(request, "store", "system"))
	domain := strings.TrimSpace(mcp.ParseString(request, "domain", "_"))

	if err := svc.CredentialDelete(ctx, store, domain, id); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete credential: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Credential %q deleted.", id)), nil
}

// --- jenkins_view_create ---

func (j JenkinsWrite) ViewCreateDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_view_create",
		mcp.WithDescription(fmt.Sprintf(
			"Create a Jenkins view from XML configuration. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("View name"),
		),
		mcp.WithString("config_xml",
			mcp.Required(),
			mcp.Description("Jenkins view XML configuration"),
		),
	)
}

func (j JenkinsWrite) ViewCreateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	name := strings.TrimSpace(mcp.ParseString(request, "name", ""))
	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}
	configXML := mcp.ParseString(request, "config_xml", "")
	if strings.TrimSpace(configXML) == "" {
		return mcp.NewToolResultError("config_xml parameter is required"), nil
	}
	version, err := svc.ViewCreate(ctx, env, name, configXML)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create view: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("View %q created (snapshot v%d).", name, version)), nil
}

// --- jenkins_view_delete ---

func (j JenkinsWrite) ViewDeleteDefinition() mcp.Tool {
	return mcp.NewTool("jenkins_view_delete",
		mcp.WithDescription(fmt.Sprintf(
			"Delete a Jenkins view. Available environments: %s.",
			j.envList(),
		)),
		mcp.WithString("environment",
			mcp.Required(),
			mcp.Description(fmt.Sprintf("Jenkins environment name: %s", j.envList())),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("View name to delete"),
		),
	)
}

func (j JenkinsWrite) ViewDeleteHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	env := parseEnv(request)
	if env == "" {
		return mcp.NewToolResultError("environment parameter is required"), nil
	}
	svc, err := j.resolveService(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	name := strings.TrimSpace(mcp.ParseString(request, "name", ""))
	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}
	if err := svc.ViewDelete(ctx, name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete view: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("View %q deleted.", name)), nil
}

// jsonUnmarshal decodes a JSON string into v.
func jsonUnmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}
