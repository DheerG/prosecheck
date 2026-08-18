package modelruntime

import (
	"fmt"
	"runtime"
)

const (
	ModelName        = "ministral-3-8b"
	ModelFileName    = "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf"
	ModelSHA256      = "33e7a72cf5e6e2cfc2f2847075acc013d68bba023e35310cef86b5cf8fdca761"
	ModelSize        = int64(5198911904)
	ModelRevision    = "0102285ad796bd99af90f58de616092e5630e970"
	RuntimeVersion   = "prism-b9599-9ca265a"
	DefaultPort      = 11435
	defaultContext   = 8192
	modelDownloadURL = "https://huggingface.co/mistralai/Ministral-3-8B-Instruct-2512-GGUF/resolve/" + ModelRevision + "/" + ModelFileName + "?download=true"
)

type runtimeAsset struct {
	URL       string
	SHA256    string
	Archive   string
	ServerExe string
}

var runtimeAssets = map[string]runtimeAsset{
	"darwin/arm64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-prism-b9599-9ca265a-bin-macos-arm64.tar.gz",
		SHA256:    "0452e5ffbab947b54cb03abbafe3ab9e4745e0aba2d9fabcf83440349e7b1cd0",
		Archive:   "tar.gz",
		ServerExe: "llama-server",
	},
	"darwin/amd64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-prism-b9599-9ca265a-bin-macos-x64.tar.gz",
		SHA256:    "7c6179a43265c0f24c7336c976b5aae81589b33cb979543421d538ae25175d88",
		Archive:   "tar.gz",
		ServerExe: "llama-server",
	},
	"linux/amd64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-prism-b9599-9ca265a-bin-ubuntu-x64.tar.gz",
		SHA256:    "35e935c828f58f9837d3da357a86846deb11d4feffc835d9e94e7d8fc1bbd55e",
		Archive:   "tar.gz",
		ServerExe: "llama-server",
	},
	"linux/arm64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-prism-b9599-9ca265a-bin-ubuntu-arm64.tar.gz",
		SHA256:    "885679964ff476cb626f2e70d593f9a12c5fedb253260dc100d31df44ba68b81",
		Archive:   "tar.gz",
		ServerExe: "llama-server",
	},
	"windows/amd64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-bin-win-cpu-x64.zip",
		SHA256:    "9253ff142d5d08cc4f80e22893a44a80e088ae0589dfa47acc2e5548a4f6818f",
		Archive:   "zip",
		ServerExe: "llama-server.exe",
	},
	"windows/arm64": {
		URL:       "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b9599-9ca265a/llama-bin-win-cpu-arm64.zip",
		SHA256:    "e88c0d35344c052cae97ccff921508e18bdc9995c6f88bf790f3366fade192f1",
		Archive:   "zip",
		ServerExe: "llama-server.exe",
	},
}

func currentAsset() (runtimeAsset, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	asset, ok := runtimeAssets[key]
	if !ok {
		return runtimeAsset{}, fmt.Errorf("the managed model does not support %s", key)
	}
	return asset, nil
}
