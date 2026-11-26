package services

import (
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/version"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const appVersion = "0.2.1"
const distribution = "https://raw.githubusercontent.com/chanpej/unityperf/"
const maxBinary = 128 << 20

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

type updateManifest struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func (p *PerfService) CheckForUpdate() (string, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return "", fmt.Errorf("当前更新包适用于 Windows x64")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return updateExecutable(&http.Client{Timeout: 2 * time.Minute}, distribution, exe, appVersion)
}

func fetchUpdate(client *http.Client, url string) (io.ReadCloser, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub 返回 %s", resp.Status)
	}
	return resp.Body, nil
}

func updateExecutable(client *http.Client, base, target, current string) (string, error) {
	body, err := fetchUpdate(client, base+"main/update.json")
	if err != nil {
		return "", err
	}
	defer body.Close()
	var release updateManifest
	if err = json.NewDecoder(io.LimitReader(body, 4096)).Decode(&release); err != nil {
		return "", err
	}
	digest, err := hex.DecodeString(release.SHA256)
	if !releaseVersion.MatchString(release.Version) || err != nil || len(digest) != sha256.Size {
		return "", fmt.Errorf("GitHub 更新清单无效")
	}
	if version.Compare("go"+release.Version, "go"+current) <= 0 {
		return "当前已是最新版本 v" + current, nil
	}
	unlock, err := lockUpdate(target)
	if err != nil {
		return "", err
	}
	defer unlock()
	installed, err := os.Open(target)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, installed)
	installed.Close()
	if err != nil {
		return "", err
	}
	ready := "已更新至 v" + release.Version + "，下次启动生效"
	if strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), release.SHA256) {
		return ready, nil
	}
	bodyEXE, err := fetchUpdate(client, base+"v"+release.Version+"/bin/unityperf.exe")
	if err != nil {
		return "", err
	}
	defer bodyEXE.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".unityperf-update-*.exe")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	hash.Reset()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(bodyEXE, maxBinary+1))
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size > maxBinary || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), release.SHA256) {
		return "", fmt.Errorf("更新文件校验失败，请稍后重试")
	}
	image, err := pe.Open(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("无效的 Windows 更新文件：%w", err)
	}
	machine := image.Machine
	image.Close()
	if machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return "", fmt.Errorf("更新文件架构不匹配")
	}
	backup := target + ".update-old"
	if err = os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("请退出旧版本后重试：%w", err)
	}
	if err = os.Rename(target, backup); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		if restore := os.Rename(backup, target); restore != nil {
			return "", fmt.Errorf("替换失败 %v，旧版本保存在 %s：%w", err, backup, restore)
		}
		return "", err
	}
	// Windows may retain the running image until its process exits.
	_ = os.Remove(backup)
	return ready, nil
}
