package service

import (
	"bytes"
	"context"
	"encoding/json"
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/pkg"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	releaseRepository = "kukumi1/flux-panel-plus"
	containerRegistry = "ghcr.io/kukumi1/flux-panel-plus"
)

var releaseVersionPattern = regexp.MustCompile(`^[0-9A-Za-z._-]+$`)

var (
	updateCache     *UpdateResult
	updateCacheMu   sync.Mutex
	updateCacheTime time.Time
	cacheTTL        = 1 * time.Hour
)

type UpdateResult struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	HasUpdate  bool   `json:"hasUpdate"`
	ReleaseURL string `json:"releaseUrl"`
}

// getLatestRelease fetches the latest release tag and version from GitHub.
func getLatestRelease() (version string, tag string, err error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + releaseRepository + "/releases/latest")
	if err != nil {
		return "", "", fmt.Errorf("检查更新失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("检查更新失败: GitHub API 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("读取更新信息失败")
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", "", fmt.Errorf("解析更新信息失败")
	}
	if release.TagName == "" {
		return "", "", fmt.Errorf("检查更新失败: 最新版本标签为空")
	}

	version = strings.TrimPrefix(release.TagName, "v")
	if !releaseVersionPattern.MatchString(version) {
		return "", "", fmt.Errorf("检查更新失败: 版本标签格式无效")
	}
	return version, release.TagName, nil
}

func CheckUpdate() dto.R {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	if updateCache != nil && time.Since(updateCacheTime) < cacheTTL {
		return dto.Ok(updateCache)
	}

	return checkUpdateNoCache()
}

// ForceCheckUpdate bypasses cache and always fetches latest from GitHub.
func ForceCheckUpdate() dto.R {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	return checkUpdateNoCache()
}

// checkUpdateNoCache fetches the latest release and updates the cache.
// Caller must hold updateCacheMu.
func checkUpdateNoCache() dto.R {
	latestVersion, tag, err := getLatestRelease()
	if err != nil {
		return dto.Err(err.Error())
	}

	currentVersion := strings.TrimPrefix(pkg.Version, "v")
	result := &UpdateResult{
		Current:    pkg.Version,
		Latest:     tag,
		HasUpdate:  currentVersion != "dev" && currentVersion != latestVersion,
		ReleaseURL: fmt.Sprintf("https://github.com/%s/releases/tag/%s", releaseRepository, tag),
	}

	updateCache = result
	updateCacheTime = time.Now()

	return dto.Ok(result)
}

// dockerRequest sends a request to the Docker Engine API via unix socket.
func dockerRequest(method, path string, body interface{}) ([]byte, error) {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, "http://localhost"+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Docker API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// getComposeInfo reads the compose working directory and project name from the backend container's labels.
func getComposeInfo() (hostDir string, projectName string, err error) {
	data, err := dockerRequest("GET", "/containers/go-backend/json", nil)
	if err != nil {
		return "", "", fmt.Errorf("获取容器信息失败: %v", err)
	}

	var info struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", "", fmt.Errorf("解析容器信息失败: %v", err)
	}

	hostDir, ok := info.Config.Labels["com.docker.compose.project.working_dir"]
	if !ok || hostDir == "" {
		return "", "", fmt.Errorf("未找到 compose 工作目录 label")
	}

	projectName = info.Config.Labels["com.docker.compose.project"]
	if projectName == "" {
		projectName = "flux-panel"
	}

	return hostDir, projectName, nil
}

func SelfUpdate() dto.R {
	// 1. Get latest version
	latestVersion, _, err := getLatestRelease()
	if err != nil {
		return dto.Err(err.Error())
	}

	current := strings.TrimPrefix(pkg.Version, "v")
	if current == "dev" {
		return dto.Err("开发版本不支持面板内自更新")
	}
	if latestVersion == current {
		return dto.Err("已是最新版本")
	}

	// 2. Get host compose directory and project name from container labels
	hostDir, projectName, err := getComposeInfo()
	if err != nil {
		return dto.Err(err.Error())
	}

	// 3. Pull docker:cli image
	updaterImage := "docker:cli"
	pullClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 5 * time.Minute,
	}
	pullReq, _ := http.NewRequest("POST", "http://localhost/images/create?fromImage=docker&tag=cli", nil)
	pullResp, pullErr := pullClient.Do(pullReq)
	if pullErr != nil {
		return dto.Err(fmt.Sprintf("拉取更新镜像失败: %v", pullErr))
	}
	io.Copy(io.Discard, pullResp.Body)
	pullResp.Body.Close()
	if pullResp.StatusCode >= 400 {
		return dto.Err(fmt.Sprintf("拉取更新镜像失败，状态码: %d", pullResp.StatusCode))
	}

	// 4. Create updater container
	// The updater mounts the host compose directory and does all the work:
	// - updates FLUX_VERSION in .env
	// - docker compose pull + up -d restarts with new images
	// This avoids needing a bind mount on the backend container.
	versionCmd := fmt.Sprintf(`if [ -f .env ] && grep -q '^FLUX_VERSION=' .env; then sed -i 's/^FLUX_VERSION=.*/FLUX_VERSION=%s/' .env; else printf '\nFLUX_VERSION=%s\n' >> .env; fi`, latestVersion, latestVersion)
	// Mount host compose dir at the SAME path inside the updater container.
	// This ensures `docker compose up -d` sets the working_dir label to the
	// real host path, so subsequent updates can find the compose file.
	updaterCmd := fmt.Sprintf("sleep 3 && cd '%s' && cp docker-compose.yml docker-compose.yml.bak && ( [ ! -f .env ] || cp .env .env.bak ) && %s && docker compose -p %s pull && docker compose -p %s up -d",
		hostDir, versionCmd, projectName, projectName)

	createBody := map[string]interface{}{
		"Image": updaterImage,
		"Cmd":   []string{"sh", "-c", updaterCmd},
		"HostConfig": map[string]interface{}{
			"Binds": []string{"/var/run/docker.sock:/var/run/docker.sock", hostDir + ":" + hostDir},
		},
	}

	respData, err := dockerRequest("POST", "/containers/create?name=flux-updater", createBody)
	if err != nil {
		// If container name conflicts, remove old one and retry
		if strings.Contains(err.Error(), "Conflict") {
			dockerRequest("DELETE", "/containers/flux-updater?force=true", nil)
			respData, err = dockerRequest("POST", "/containers/create?name=flux-updater", createBody)
		}
		if err != nil {
			return dto.Err(fmt.Sprintf("创建更新容器失败: %v", err))
		}
	}

	var createResp struct {
		Id string `json:"Id"`
	}
	if err := json.Unmarshal(respData, &createResp); err != nil {
		return dto.Err(fmt.Sprintf("解析容器ID失败: %v", err))
	}

	// Start the updater container
	if _, err := dockerRequest("POST", "/containers/"+createResp.Id+"/start", nil); err != nil {
		return dto.Err(fmt.Sprintf("启动更新容器失败: %v", err))
	}

	// Clear update cache so next check reflects new state
	updateCacheMu.Lock()
	updateCache = nil
	updateCacheMu.Unlock()

	return dto.Ok("更新已启动，面板将在几秒后自动重启")
}
