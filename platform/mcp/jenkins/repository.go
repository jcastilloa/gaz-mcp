// Package jenkins implements the domain.Repository interface using gojenkins.
package jenkins

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/bndr/gojenkins"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"
)

// Repository implements domain.Repository backed by gojenkins.
type Repository struct {
	client     *gojenkins.Jenkins
	httpClient *http.Client
	baseURL    string
	user       string
	apiToken   string
}

// NewRepository creates a new Jenkins repository and initializes the connection.
func NewRepository(cfg configDomain.JenkinsEnvironmentConfig) (*Repository, error) {
	rawURL := strings.TrimRight(cfg.URL, "/") + "/"

	httpClient := &http.Client{}
	if cfg.Insecure {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	if cfg.Timeout > 0 {
		httpClient.Timeout = cfg.Timeout
	}

	client := gojenkins.CreateJenkins(httpClient, rawURL, cfg.User, cfg.APIKey)

	ctx := context.Background()
	if _, err := client.Init(ctx); err != nil {
		return nil, fmt.Errorf("init jenkins client: %w", err)
	}

	return &Repository{
		client:     client,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		user:       cfg.User,
		apiToken:   cfg.APIKey,
	}, nil
}

// rawGet performs a plain HTTP GET against the Jenkins base URL + path,
// returning the response body as a string. Used for endpoints that return
// plain text (e.g. /consoleText) instead of JSON.
func (r *Repository) rawGet(ctx context.Context, path string) (string, error) {
	reqURL := r.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(r.user, r.apiToken)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response %s: %w", path, err)
	}
	return string(body), nil
}

// --- System ---

// infoRaw is the shape returned by Jenkins /api/json?tree=...
type infoRaw struct {
	QuietingDown bool `json:"quietingDown"`
	Jobs         []struct {
		Name string `json:"name"`
	} `json:"jobs"`
	Computers []struct {
		DisplayName string `json:"displayName"`
	} `json:"computers"`
}

func (r *Repository) Info(ctx context.Context) (*domain.JenkinsInfo, error) {
	const tree = "quietingDown,jobs[name],computers[displayName]"
	var raw infoRaw
	if _, err := r.client.Requester.GetJSON(ctx, "/api/json", &raw, map[string]string{"tree": tree}); err != nil {
		return nil, fmt.Errorf("jenkins info: %w", err)
	}

	return &domain.JenkinsInfo{
		Version:      r.client.Version,
		JobCount:     len(raw.Jobs),
		NodeCount:    len(raw.Computers),
		QuietingDown: raw.QuietingDown,
	}, nil
}

func (r *Repository) QuietDown(ctx context.Context) error {
	if _, err := r.client.Requester.Post(ctx, "/quietDown", nil, nil, nil); err != nil {
		return fmt.Errorf("quiet down: %w", err)
	}
	return nil
}

func (r *Repository) CancelQuietDown(ctx context.Context) error {
	if _, err := r.client.Requester.Post(ctx, "/cancelQuietDown", nil, nil, nil); err != nil {
		return fmt.Errorf("cancel quiet down: %w", err)
	}
	return nil
}

// --- Jobs ---

// jobListRaw is the shape returned by Jenkins /api/json?tree=jobs[...].
type jobListRaw struct {
	Jobs []jobRaw `json:"jobs"`
}

type jobRaw struct {
	Name         string      `json:"name"`
	URL          string      `json:"url"`
	Color        string      `json:"color"`
	Description  string      `json:"description"`
	LastBuild    *buildBrief `json:"lastBuild"`
	HealthReport []struct {
		Score       int    `json:"score"`
		Description string `json:"description"`
	} `json:"healthReport"`
}

type buildBrief struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func (r *Repository) JobList(ctx context.Context, filter string) ([]domain.JobInfo, error) {
	// Use tree parameter to fetch all job data in a single HTTP request.
	const tree = "jobs[name,url,color,description,lastBuild[number,url],healthReport[score,description]]"
	var raw jobListRaw
	if _, err := r.client.Requester.GetJSON(ctx, "/api/json", &raw, map[string]string{"tree": tree}); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	result := make([]domain.JobInfo, 0, len(raw.Jobs))
	for _, j := range raw.Jobs {
		info := domain.JobInfo{
			Name:        j.Name,
			URL:         j.URL,
			Color:       j.Color,
			Description: j.Description,
		}
		if j.LastBuild != nil && j.LastBuild.Number > 0 {
			info.LastBuild = &domain.BuildBrief{
				Number: j.LastBuild.Number,
				URL:    j.LastBuild.URL,
			}
		}
		for _, h := range j.HealthReport {
			info.Health = append(info.Health, domain.HealthReport{
				Score:       h.Score,
				Description: h.Description,
			})
		}
		if filter == "" || strings.Contains(strings.ToLower(info.Name), strings.ToLower(filter)) {
			result = append(result, info)
		}
	}

	sort.Slice(result, func(i, k int) bool {
		return result[i].Name < result[k].Name
	})

	return result, nil
}

func (r *Repository) JobGet(ctx context.Context, name string) (*domain.JobInfo, error) {
	job, err := r.client.GetJob(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get job %q: %w", name, err)
	}
	job.Poll(ctx) //nolint:errcheck

	innerJobs, _ := job.GetInnerJobs(ctx)
	info := jobToInfo(job)
	info.IsFolder = len(innerJobs) > 0

	return &info, nil
}

func (r *Repository) JobConfig(ctx context.Context, name string) (string, error) {
	job, err := r.client.GetJob(ctx, name)
	if err != nil {
		return "", fmt.Errorf("get job config %q: %w", name, err)
	}
	cfg, err := job.GetConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("get config for %q: %w", name, err)
	}
	return cfg, nil
}

func (r *Repository) JobSetConfig(ctx context.Context, name, configXML string) error {
	job, err := r.client.GetJob(ctx, name)
	if err != nil {
		return fmt.Errorf("get job for update %q: %w", name, err)
	}
	if err := job.UpdateConfig(ctx, configXML); err != nil {
		return fmt.Errorf("update config for %q: %w", name, err)
	}
	return nil
}

func (r *Repository) JobCreate(ctx context.Context, name, configXML string) error {
	if _, err := r.client.CreateJob(ctx, configXML, name); err != nil {
		return fmt.Errorf("create job %q: %w", name, err)
	}
	return nil
}

func (r *Repository) JobCopy(ctx context.Context, from, to string) error {
	if _, err := r.client.CopyJob(ctx, from, to); err != nil {
		return fmt.Errorf("copy job %q to %q: %w", from, to, err)
	}
	return nil
}

func (r *Repository) JobDelete(ctx context.Context, name string) error {
	if _, err := r.client.DeleteJob(ctx, name); err != nil {
		return fmt.Errorf("delete job %q: %w", name, err)
	}
	return nil
}

func (r *Repository) JobEnable(ctx context.Context, name string) error {
	job, err := r.client.GetJob(ctx, name)
	if err != nil {
		return fmt.Errorf("get job for enable %q: %w", name, err)
	}
	if _, err := job.Enable(ctx); err != nil {
		return fmt.Errorf("enable job %q: %w", name, err)
	}
	return nil
}

func (r *Repository) JobDisable(ctx context.Context, name string) error {
	job, err := r.client.GetJob(ctx, name)
	if err != nil {
		return fmt.Errorf("get job for disable %q: %w", name, err)
	}
	if _, err := job.Disable(ctx); err != nil {
		return fmt.Errorf("disable job %q: %w", name, err)
	}
	return nil
}

func (r *Repository) JobBuild(ctx context.Context, name string, params map[string]string) (int64, error) {
	if len(params) > 0 {
		job, err := r.client.GetJob(ctx, name)
		if err != nil {
			return 0, fmt.Errorf("get job for build %q: %w", name, err)
		}
		queueID, err := job.InvokeSimple(ctx, params)
		if err != nil {
			return 0, fmt.Errorf("build job %q with params: %w", name, err)
		}
		return queueID, nil
	}

	queueID, err := r.client.BuildJob(ctx, name, nil)
	if err != nil {
		return 0, fmt.Errorf("build job %q: %w", name, err)
	}
	return queueID, nil
}

// --- Builds ---

// buildInfoRaw is the shape returned by Jenkins /job/{name}/{num}/api/json.
type buildInfoRaw struct {
	Number    int     `json:"number"`
	Result    string  `json:"result"`
	Duration  float64 `json:"duration"`
	Timestamp int64   `json:"timestamp"`
	URL       string  `json:"url"`
	Building  bool    `json:"building"`
	Actions   []struct {
		Class  string `json:"_class"`
		Causes []struct {
			ShortDescription string `json:"shortDescription"`
		} `json:"causes"`
		Parameters []struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		} `json:"parameters"`
		LastBuiltRevision *struct {
			SHA1     string `json:"SHA1"`
			Branches []struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"lastBuiltRevision"`
	} `json:"actions"`
	Artifacts []struct {
		FileName string `json:"fileName"`
		RelPath  string `json:"relativePath"`
	} `json:"artifacts"`
}

func (r *Repository) buildPath(jobName string, buildNum int) string {
	return fmt.Sprintf("/job/%s/%d", url.PathEscape(jobName), buildNum)
}

func (r *Repository) BuildInfo(ctx context.Context, jobName string, buildNum int) (*domain.BuildInfo, error) {
	var raw buildInfoRaw
	endpoint := r.buildPath(jobName, buildNum) + "/api/json"
	if _, err := r.client.Requester.GetJSON(ctx, endpoint, &raw, nil); err != nil {
		return nil, fmt.Errorf("get build %d for %q: %w", buildNum, jobName, err)
	}

	var causes []string
	params := make(map[string]string)
	var revision, branch string

	for _, action := range raw.Actions {
		for _, c := range action.Causes {
			if c.ShortDescription != "" {
				causes = append(causes, c.ShortDescription)
			}
		}
		for _, p := range action.Parameters {
			params[p.Name] = fmt.Sprintf("%v", p.Value)
		}
		if action.LastBuiltRevision != nil {
			revision = action.LastBuiltRevision.SHA1
			if len(action.LastBuiltRevision.Branches) > 0 {
				branch = action.LastBuiltRevision.Branches[0].Name
			}
		}
	}

	return &domain.BuildInfo{
		Number:     raw.Number,
		Result:     raw.Result,
		Duration:   int64(raw.Duration),
		Timestamp:  raw.Timestamp,
		URL:        raw.URL,
		Building:   raw.Building,
		Causes:     causes,
		Parameters: params,
		Revision:   revision,
		Branch:     branch,
	}, nil
}

func (r *Repository) buildConsoleText(ctx context.Context, jobName string, buildNum int) (string, error) {
	path := fmt.Sprintf("/job/%s/%d/consoleText", url.PathEscape(jobName), buildNum)
	text, err := r.rawGet(ctx, path)
	if err != nil {
		return "", fmt.Errorf("get build log %d for %q: %w", buildNum, jobName, err)
	}
	return text, nil
}

func (r *Repository) BuildLog(ctx context.Context, jobName string, buildNum int, startLine int) (string, int, error) {
	logText, err := r.buildConsoleText(ctx, jobName, buildNum)
	if err != nil {
		return "", 0, err
	}

	lines := strings.Split(logText, "\n")
	totalLines := len(lines)
	if startLine >= totalLines {
		return "", totalLines, nil
	}
	return strings.Join(lines[startLine:], "\n"), totalLines, nil
}

func (r *Repository) BuildLogProgressive(ctx context.Context, jobName string, buildNum int) (string, error) {
	return r.buildConsoleText(ctx, jobName, buildNum)
}

func (r *Repository) BuildStop(ctx context.Context, jobName string, buildNum int) error {
	endpoint := r.buildPath(jobName, buildNum) + "/stop"
	if _, err := r.client.Requester.Post(ctx, endpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("stop build %d for %q: %w", buildNum, jobName, err)
	}
	return nil
}

func (r *Repository) BuildDelete(ctx context.Context, jobName string, buildNum int) error {
	endpoint := r.buildPath(jobName, buildNum) + "/doDelete"
	if _, err := r.client.Requester.Post(ctx, endpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("delete build %d for %q: %w", buildNum, jobName, err)
	}
	return nil
}

func (r *Repository) BuildArtifacts(ctx context.Context, jobName string, buildNum int) ([]domain.Artifact, error) {
	var raw buildInfoRaw
	endpoint := r.buildPath(jobName, buildNum) + "/api/json"
	if _, err := r.client.Requester.GetJSON(ctx, endpoint, &raw, nil); err != nil {
		return nil, fmt.Errorf("get build artifacts %d for %q: %w", buildNum, jobName, err)
	}

	result := make([]domain.Artifact, 0, len(raw.Artifacts))
	for _, a := range raw.Artifacts {
		result = append(result, domain.Artifact{
			Name: a.FileName,
			Path: a.RelPath,
		})
	}
	return result, nil
}

// --- Nodes ---

// nodeListRaw is the shape returned by Jenkins /computer/api/json?tree=...
type nodeListRaw struct {
	Computers []nodeRaw `json:"computer"`
}

type nodeRaw struct {
	DisplayName string `json:"displayName"`
	Offline     bool   `json:"offline"`
	Idle        bool   `json:"idle"`
}

func (r *Repository) NodeList(ctx context.Context) ([]domain.NodeInfo, error) {
	const tree = "computer[displayName,offline,idle]"
	var raw nodeListRaw
	if _, err := r.client.Requester.GetJSON(ctx, "/computer/api/json", &raw, map[string]string{"tree": tree}); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	result := make([]domain.NodeInfo, 0, len(raw.Computers))
	for _, n := range raw.Computers {
		result = append(result, domain.NodeInfo{
			Name:        n.DisplayName,
			DisplayName: n.DisplayName,
			Online:      !n.Offline,
			Idle:        n.Idle,
		})
	}

	return result, nil
}

func (r *Repository) NodeGet(ctx context.Context, name string) (*domain.NodeInfo, error) {
	node, err := r.client.GetNode(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get node %q: %w", name, err)
	}
	node.Poll(ctx) //nolint:errcheck

	info := nodeToInfo(ctx, node)
	return &info, nil
}

func (r *Repository) NodeCreate(ctx context.Context, name, configXML string) error {
	endpoint := fmt.Sprintf("/computer/doCreateItem?name=%s&type=hudson.slaves.DumbSlave", url.QueryEscape(name))
	if _, err := r.client.Requester.PostXML(ctx, endpoint, configXML, nil, nil); err != nil {
		return fmt.Errorf("create node %q: %w", name, err)
	}
	return nil
}

func (r *Repository) NodeDelete(ctx context.Context, name string) error {
	if _, err := r.client.DeleteNode(ctx, name); err != nil {
		return fmt.Errorf("delete node %q: %w", name, err)
	}
	return nil
}

func (r *Repository) NodeEnable(ctx context.Context, name string) error {
	node, err := r.client.GetNode(ctx, name)
	if err != nil {
		return fmt.Errorf("get node for enable %q: %w", name, err)
	}
	if _, err := node.SetOnline(ctx); err != nil {
		return fmt.Errorf("enable node %q: %w", name, err)
	}
	return nil
}

func (r *Repository) NodeDisable(ctx context.Context, name string, message string) error {
	node, err := r.client.GetNode(ctx, name)
	if err != nil {
		return fmt.Errorf("get node for disable %q: %w", name, err)
	}
	if _, err := node.SetOffline(ctx, message); err != nil {
		return fmt.Errorf("disable node %q: %w", name, err)
	}
	return nil
}

func (r *Repository) NodeDisconnect(ctx context.Context, name string, _ string) error {
	node, err := r.client.GetNode(ctx, name)
	if err != nil {
		return fmt.Errorf("get node for disconnect %q: %w", name, err)
	}
	if _, err := node.Disconnect(ctx); err != nil {
		return fmt.Errorf("disconnect node %q: %w", name, err)
	}
	return nil
}

// --- Views ---

// viewListRaw is the shape returned by Jenkins /api/json?tree=views[...]
type viewListRaw struct {
	Views []viewRaw `json:"views"`
}

type viewRaw struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Jobs []struct {
		Name string `json:"name"`
	} `json:"jobs"`
}

func (r *Repository) ViewList(ctx context.Context) ([]domain.ViewInfo, error) {
	const tree = "views[name,url,jobs[name]]"
	var raw viewListRaw
	if _, err := r.client.Requester.GetJSON(ctx, "/api/json", &raw, map[string]string{"tree": tree}); err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}

	result := make([]domain.ViewInfo, 0, len(raw.Views))
	for _, v := range raw.Views {
		info := domain.ViewInfo{
			Name: v.Name,
			URL:  v.URL,
		}
		for _, j := range v.Jobs {
			info.Jobs = append(info.Jobs, j.Name)
		}
		result = append(result, info)
	}

	return result, nil
}

func (r *Repository) ViewGet(ctx context.Context, name string) (*domain.ViewInfo, error) {
	view, err := r.client.GetView(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get view %q: %w", name, err)
	}
	view.Poll(ctx) //nolint:errcheck

	info := viewToInfo(view)
	return &info, nil
}

func (r *Repository) ViewCreate(ctx context.Context, name, configXML string) error {
	// gojenkins CreateView only accepts a view type string, not XML.
	// Use raw REST API to POST the XML config.
	endpoint := fmt.Sprintf("/createView?name=%s", url.QueryEscape(name))
	if _, err := r.client.Requester.PostXML(ctx, endpoint, configXML, nil, nil); err != nil {
		return fmt.Errorf("create view %q: %w", name, err)
	}
	return nil
}

func (r *Repository) ViewDelete(ctx context.Context, name string) error {
	// gojenkins has no DeleteView method; use raw REST API.
	endpoint := fmt.Sprintf("/view/%s/doDelete", url.PathEscape(name))
	if _, err := r.client.Requester.Post(ctx, endpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("delete view %q: %w", name, err)
	}
	return nil
}

func (r *Repository) ViewAddJob(ctx context.Context, viewName, jobName string) error {
	view, err := r.client.GetView(ctx, viewName)
	if err != nil {
		return fmt.Errorf("get view %q: %w", viewName, err)
	}
	if _, err := view.AddJob(ctx, jobName); err != nil {
		return fmt.Errorf("add job %q to view %q: %w", jobName, viewName, err)
	}
	return nil
}

func (r *Repository) ViewRemoveJob(ctx context.Context, viewName, jobName string) error {
	view, err := r.client.GetView(ctx, viewName)
	if err != nil {
		return fmt.Errorf("get view %q: %w", viewName, err)
	}
	if _, err := view.DeleteJob(ctx, jobName); err != nil {
		return fmt.Errorf("remove job %q from view %q: %w", jobName, viewName, err)
	}
	return nil
}

// --- Queue ---

func (r *Repository) QueueList(ctx context.Context) ([]domain.QueueItem, error) {
	queue, err := r.client.GetQueue(ctx)
	if err != nil {
		return nil, fmt.Errorf("get queue: %w", err)
	}

	tasks := queue.Tasks()
	result := make([]domain.QueueItem, 0, len(tasks))
	for _, t := range tasks {
		if t.Raw == nil {
			continue
		}
		result = append(result, domain.QueueItem{
			ID:           t.Raw.ID,
			Task:         t.Raw.Task.Name,
			URL:          t.Raw.URL,
			Why:          t.Raw.Why,
			Blocked:      t.Raw.Blocked,
			Buildable:    t.Raw.Buildable,
			InQueueSince: t.Raw.InQueueSince,
		})
	}

	return result, nil
}

func (r *Repository) QueueCancel(ctx context.Context, id int64) error {
	queue, err := r.client.GetQueue(ctx)
	if err != nil {
		return fmt.Errorf("get queue: %w", err)
	}
	if _, err := queue.CancelTask(ctx, id); err != nil {
		return fmt.Errorf("cancel queue item %d: %w", id, err)
	}
	return nil
}

// --- Plugins ---

func (r *Repository) PluginList(ctx context.Context) ([]domain.PluginInfo, error) {
	plugins, err := r.client.GetPlugins(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}

	result := make([]domain.PluginInfo, 0, plugins.Count())
	for _, p := range plugins.Raw.Plugins {
		result = append(result, domain.PluginInfo{
			ShortName: p.ShortName,
			Version:   p.Version,
			Enabled:   p.Enabled,
		})
	}

	sort.Slice(result, func(i, k int) bool {
		return result[i].ShortName < result[k].ShortName
	})

	return result, nil
}

// --- Credentials ---

// CredentialList lists credentials in a store/domain via raw REST API.
// Jenkins credentials API: GET /credentials/store/{store}/domain/{domain}/api/json
func (r *Repository) CredentialList(ctx context.Context, store, storeDomain string) ([]domain.CredentialInfo, error) {
	endpoint := fmt.Sprintf("/credentials/store/%s/domain/%s/api/json?depth=1",
		url.PathEscape(store), url.PathEscape(storeDomain))

	var response struct {
		Credentials []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			TypeName    string `json:"typeName"`
			Description string `json:"description"`
		} `json:"credentials"`
	}

	if _, err := r.client.Requester.GetJSON(ctx, endpoint, &response, nil); err != nil {
		return nil, fmt.Errorf("list credentials store=%s domain=%s: %w", store, storeDomain, err)
	}

	result := make([]domain.CredentialInfo, 0, len(response.Credentials))
	for _, c := range response.Credentials {
		result = append(result, domain.CredentialInfo{
			ID:          c.ID,
			Name:        c.DisplayName,
			Type:        c.TypeName,
			Description: c.Description,
			Domain:      storeDomain,
		})
	}

	return result, nil
}

// CredentialCreate creates a credential via raw REST API.
func (r *Repository) CredentialCreate(ctx context.Context, store, storeDomain, _ string, configXML string) error {
	endpoint := fmt.Sprintf("/credentials/store/%s/domain/%s/createCredentials",
		url.PathEscape(store), url.PathEscape(storeDomain))

	if _, err := r.client.Requester.PostXML(ctx, endpoint, configXML, nil, nil); err != nil {
		return fmt.Errorf("create credential in store=%s domain=%s: %w", store, storeDomain, err)
	}
	return nil
}

// CredentialDelete deletes a credential via raw REST API.
func (r *Repository) CredentialDelete(ctx context.Context, store, storeDomain, id string) error {
	endpoint := fmt.Sprintf("/credentials/store/%s/domain/%s/credential/%s/doDelete",
		url.PathEscape(store), url.PathEscape(storeDomain), url.PathEscape(id))

	if _, err := r.client.Requester.Post(ctx, endpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("delete credential %q in store=%s domain=%s: %w", id, store, storeDomain, err)
	}
	return nil
}

// --- Script Console ---

// ScriptConsole executes a Groovy script via the Jenkins script console.
func (r *Repository) ScriptConsole(ctx context.Context, script string) (string, error) {
	payload := strings.NewReader("script=" + url.QueryEscape(script))
	resp, err := r.client.Requester.Post(ctx, "/scriptText", payload, nil, nil)
	if err != nil {
		return "", fmt.Errorf("script console: %w", err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read script console response: %w", err)
		}
		return string(body), nil
	}
	return "", nil
}

// --- Helper converters ---

func jobToInfo(j *gojenkins.Job) domain.JobInfo {
	raw := j.Raw
	if raw == nil {
		return domain.JobInfo{Name: j.GetName()}
	}

	info := domain.JobInfo{
		Name:        raw.Name,
		URL:         raw.URL,
		Color:       raw.Color,
		Description: raw.Description,
	}

	// Last build (JobBuild is a value type with Number int64 and URL string)
	if raw.LastBuild.Number > 0 {
		info.LastBuild = &domain.BuildBrief{
			Number: int(raw.LastBuild.Number),
			URL:    raw.LastBuild.URL,
		}
	}

	// Health reports
	for _, h := range raw.HealthReport {
		info.Health = append(info.Health, domain.HealthReport{
			Score:       int(h.Score),
			Description: h.Description,
		})
	}

	return info
}

func nodeToInfo(ctx context.Context, n *gojenkins.Node) domain.NodeInfo {
	raw := n.Raw
	displayName := ""
	if raw != nil {
		displayName = raw.DisplayName
	}

	online, _ := n.IsOnline(ctx)
	idle, _ := n.IsIdle(ctx)

	return domain.NodeInfo{
		Name:        n.GetName(),
		DisplayName: displayName,
		Online:      online,
		Idle:        idle,
	}
}

func viewToInfo(v *gojenkins.View) domain.ViewInfo {
	info := domain.ViewInfo{
		Name: v.GetName(),
		URL:  v.GetUrl(),
	}

	for _, j := range v.GetJobs() {
		info.Jobs = append(info.Jobs, j.Name)
	}

	return info
}
