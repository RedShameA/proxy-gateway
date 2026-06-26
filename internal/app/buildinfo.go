package app

type BuildInfo struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Source   string `json:"source"`
	License  string `json:"license"`
}

var buildInfo = BuildInfo{
	Version:  "dev",
	Revision: "unknown",
	Source:   "https://github.com/RedShameA/proxy-gateway",
	License:  "AGPL-3.0-or-later",
}

func SetBuildInfo(info BuildInfo) {
	if info.Version != "" {
		buildInfo.Version = info.Version
	}
	if info.Revision != "" {
		buildInfo.Revision = info.Revision
	}
	if info.Source != "" {
		buildInfo.Source = info.Source
	}
	if info.License != "" {
		buildInfo.License = info.License
	}
}

func CurrentBuildInfo() BuildInfo {
	return buildInfo
}
