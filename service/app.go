package service

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type AppService struct {
}

type BuildInfo struct {
	Version      string `json:"version"`
	ServerCommit string `json:"server_commit"`
	APICommit    string `json:"api_commit"`
	WebCommit    string `json:"web_commit"`
	Image        string `json:"image"`
	BuiltAt      string `json:"built_at"`
	Source       string `json:"source"`
}

var version = ""
var startTime = ""
var once = &sync.Once{}

func (a *AppService) GetAppVersion() string {
	if version != "" {
		return version
	}
	once.Do(func() {
		v, err := os.ReadFile("resources/version")
		if err != nil {
			return
		}
		version = string(v)

	})
	return version
}

func (a *AppService) GetBuildInfo() BuildInfo {
	path := os.Getenv("RUSTDESK_API_BUILD_INFO_FILE")
	if path == "" {
		path = "/etc/rustdesk-full-s6-build.json"
	}
	return a.GetBuildInfoFromPath(path)
}

func (a *AppService) GetBuildInfoFromPath(path string) BuildInfo {
	info := BuildInfo{Version: a.GetAppVersion()}
	raw, err := os.ReadFile(path)
	if err != nil {
		return info
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return BuildInfo{Version: a.GetAppVersion()}
	}
	info.Version = a.GetAppVersion()
	info.Source = path
	return info
}

func init() {
	// Initialize the AppService if needed
	startTime = time.Now().Format("2006-01-02 15:04:05")
}

// GetStartTime
func (a *AppService) GetStartTime() string {
	return startTime
}
