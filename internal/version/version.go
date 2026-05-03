// Package version 提供版本信息
package version

var (
	// Version 版本号
	Version = "v1.0.0"
	// BuildTime 构建时间
	BuildTime = "unknown"
	// GoVersion Go版本
	GoVersion = "go1.22.0"
)

// Info 版本信息结构
type Info struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Get 返回版本信息
func Get() Info {
	return Info{
		Version:   Version,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
	}
}

// String 返回格式化的版本信息
func String() string {
	return "Hades " + Version + " (built at " + BuildTime + ")"
}
