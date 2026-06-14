// Package jenkins defines the domain port and data types for the Jenkins MCP proxy.
package jenkins

import "context"

// --- Domain types ---

// JenkinsInfo holds general information about a Jenkins instance.
type JenkinsInfo struct {
	Version      string `json:"version"`
	Uptime       string `json:"uptime,omitempty"`
	JobCount     int    `json:"job_count"`
	NodeCount    int    `json:"node_count"`
	QuietingDown bool   `json:"quieting_down"`
	SLabel       string `json:"slabel,omitempty"`
}

// JobInfo represents a Jenkins job.
type JobInfo struct {
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	Color       string         `json:"color"` // blue, red, yellow, disabled, notbuilt, aborted
	Description string         `json:"description,omitempty"`
	Folder      string         `json:"folder,omitempty"`
	IsFolder    bool           `json:"is_folder"`
	LastBuild   *BuildBrief    `json:"last_build,omitempty"`
	Health      []HealthReport `json:"health,omitempty"`
}

// BuildBrief summarizes a build.
type BuildBrief struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Result string `json:"result"` // SUCCESS, FAILURE, ABORTED, UNSTABLE, etc.
}

// HealthReport describes a job health score.
type HealthReport struct {
	Score       int    `json:"score"`
	Description string `json:"description"`
}

// BuildInfo holds detailed information about a build.
type BuildInfo struct {
	Number     int               `json:"number"`
	Result     string            `json:"result"`
	Duration   int64             `json:"duration_ms"`
	Timestamp  int64             `json:"timestamp"`
	URL        string            `json:"url"`
	Building   bool              `json:"building"`
	Causes     []string          `json:"causes"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Revision   string            `json:"revision,omitempty"`
	Branch     string            `json:"branch,omitempty"`
}

// Artifact represents a build artifact.
type Artifact struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// NodeInfo represents a Jenkins node or agent.
type NodeInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name,omitempty"`
	URL          string `json:"url"`
	Online       bool   `json:"online"`
	Idle         bool   `json:"idle"`
	NumExecutors int    `json:"num_executors"`
	OfflineCause string `json:"offline_cause,omitempty"`
	Launcher     string `json:"launcher,omitempty"`
}

// ViewInfo represents a Jenkins view.
type ViewInfo struct {
	Name string   `json:"name"`
	URL  string   `json:"url"`
	Jobs []string `json:"jobs,omitempty"`
}

// QueueItem represents an item in the Jenkins build queue.
type QueueItem struct {
	ID           int64  `json:"id"`
	Task         string `json:"task"`
	URL          string `json:"url"`
	Why          string `json:"why,omitempty"`
	Blocked      bool   `json:"blocked"`
	Buildable    bool   `json:"buildable"`
	InQueueSince int64  `json:"in_queue_since,omitempty"`
}

// PluginInfo represents an installed Jenkins plugin.
type PluginInfo struct {
	ShortName string `json:"short_name"`
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
}

// CredentialInfo summarizes a stored credential (without secret values).
type CredentialInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
}

// ScriptConsoleResult holds the output of a script console execution.
type ScriptConsoleResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// --- Domain port (Repository interface) ---

// Repository defines the contract for interacting with a Jenkins instance.
type Repository interface {
	// System
	Info(ctx context.Context) (*JenkinsInfo, error)
	QuietDown(ctx context.Context) error
	CancelQuietDown(ctx context.Context) error

	// Jobs
	JobList(ctx context.Context, filter string) ([]JobInfo, error)
	JobGet(ctx context.Context, name string) (*JobInfo, error)
	JobConfig(ctx context.Context, name string) (string, error)
	JobSetConfig(ctx context.Context, name, configXML string) error
	JobCreate(ctx context.Context, name, configXML string) error
	JobCopy(ctx context.Context, from, to string) error
	JobDelete(ctx context.Context, name string) error
	JobEnable(ctx context.Context, name string) error
	JobDisable(ctx context.Context, name string) error
	JobBuild(ctx context.Context, name string, params map[string]string) (int64, error)

	// Builds
	BuildInfo(ctx context.Context, jobName string, buildNum int) (*BuildInfo, error)
	BuildLog(ctx context.Context, jobName string, buildNum int, startLine int) (string, int, error)
	BuildLogProgressive(ctx context.Context, jobName string, buildNum int) (string, error)
	BuildStop(ctx context.Context, jobName string, buildNum int) error
	BuildDelete(ctx context.Context, jobName string, buildNum int) error
	BuildArtifacts(ctx context.Context, jobName string, buildNum int) ([]Artifact, error)

	// Nodes
	NodeList(ctx context.Context) ([]NodeInfo, error)
	NodeGet(ctx context.Context, name string) (*NodeInfo, error)
	NodeCreate(ctx context.Context, name, configXML string) error
	NodeDelete(ctx context.Context, name string) error
	NodeEnable(ctx context.Context, name string) error
	NodeDisable(ctx context.Context, name string, message string) error
	NodeDisconnect(ctx context.Context, name string, message string) error

	// Views
	ViewList(ctx context.Context) ([]ViewInfo, error)
	ViewGet(ctx context.Context, name string) (*ViewInfo, error)
	ViewCreate(ctx context.Context, name, configXML string) error
	ViewDelete(ctx context.Context, name string) error
	ViewAddJob(ctx context.Context, viewName, jobName string) error
	ViewRemoveJob(ctx context.Context, viewName, jobName string) error

	// Queue
	QueueList(ctx context.Context) ([]QueueItem, error)
	QueueCancel(ctx context.Context, id int64) error

	// Plugins
	PluginList(ctx context.Context) ([]PluginInfo, error)

	// Credentials
	CredentialList(ctx context.Context, store, domain string) ([]CredentialInfo, error)
	CredentialCreate(ctx context.Context, store, domain, id, configXML string) error
	CredentialDelete(ctx context.Context, store, domain, id string) error

	// Script Console
	ScriptConsole(ctx context.Context, script string) (string, error)
}
