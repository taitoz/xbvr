package tasks

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/mholt/archiver"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/xbapps/xbvr/pkg/common"
	"github.com/xbapps/xbvr/pkg/ffprobe"
)

func CheckDependencies() {
	// Check ffprobe
	ffprobePath := filepath.Join(common.BinDir, "ffprobe")
	if runtime.GOOS == "windows" {
		ffprobePath = ffprobePath + ".exe"
	}
	if _, err := os.Stat(ffprobePath); os.IsNotExist(err) {
		log.Info("ffprobe not installed, downloading now...")
		downloadFFmpeg("ffprobe")
	}

	// Check ffmpeg
	ffmpegPath := filepath.Join(common.BinDir, "ffmpeg")
	if runtime.GOOS == "windows" {
		ffmpegPath = ffmpegPath + ".exe"
	}
	if _, err := os.Stat(ffmpegPath); os.IsNotExist(err) {
		log.Info("ffmpeg not installed, downloading now...")
		downloadFFmpeg("ffmpeg")
	}

	// Set path for go-ffprobe
	ffprobe.SetFFProbeBinPath(ffprobePath)
}

func GetBinPath(tool string) string {
	path := filepath.Join(common.BinDir, tool)
	if runtime.GOOS == "windows" {
		path = path + ".exe"
	}
	return path
}

func downloadFFmpeg(tool string) error {
	// Use BtbN/FFmpeg-Builds for win64 and linux64 (latest auto-builds)
	if (runtime.GOOS == "windows" && runtime.GOARCH == "amd64") ||
		(runtime.GOOS == "linux" && runtime.GOARCH == "amd64") ||
		(runtime.GOOS == "linux" && runtime.GOARCH == "arm64") {
		return downloadFFmpegFromBtbN(tool)
	}
	// Fallback to ffbinaries.com for other platforms (macOS, 32-bit)
	return downloadFFmpegFromFfbinaries(tool)
}

func downloadFFmpegFromBtbN(tool string) error {
	var assetPattern string
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		assetPattern = "ffmpeg-master-latest-win64-gpl.zip"
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		assetPattern = "ffmpeg-master-latest-linux64-gpl.zip"
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		assetPattern = "ffmpeg-master-latest-linuxarm64-gpl.zip"
	} else {
		return downloadFFmpegFromFfbinaries(tool)
	}

	url := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/" + assetPattern
	log.Infof("Downloading %s from BtbN/FFmpeg-Builds", tool)

	err := downloadFile(url, filepath.Join(common.BinDir, "ffmpeg-btbn.zip"))
	if err != nil {
		log.Warnf("BtbN download failed, falling back to ffbinaries.com: %v", err)
		return downloadFFmpegFromFfbinaries(tool)
	}

	// BtbN zip has a subdirectory like ffmpeg-master-latest-win64-gpl/
	// Extract to temp dir, then copy binaries to BinDir
	tmpDir := filepath.Join(common.BinDir, "_ffmpeg_extract")
	_ = os.RemoveAll(tmpDir)
	err = archiver.Unarchive(filepath.Join(common.BinDir, "ffmpeg-btbn.zip"), tmpDir)
	if err != nil {
		log.Warnf("BtbN extract failed, falling back to ffbinaries.com: %v", err)
		_ = os.RemoveAll(tmpDir)
		return downloadFFmpegFromFfbinaries(tool)
	}

	// Find and copy the tool binary from extracted subdirectory
	found := false
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if entry.IsDir() {
			srcPath := filepath.Join(tmpDir, entry.Name(), tool)
			if runtime.GOOS == "windows" {
				srcPath = srcPath + ".exe"
			}
			if _, err := os.Stat(srcPath); err == nil {
				destPath := filepath.Join(common.BinDir, tool)
				if runtime.GOOS == "windows" {
					destPath = destPath + ".exe"
				}
				copyFileLocal(srcPath, destPath)
				found = true
				break
			}
		}
	}

	_ = os.RemoveAll(tmpDir)
	_ = os.Remove(filepath.Join(common.BinDir, "ffmpeg-btbn.zip"))

	if !found {
		log.Warnf("BtbN binary not found in archive, falling back to ffbinaries.com")
		return downloadFFmpegFromFfbinaries(tool)
	}
	return nil
}

func downloadFFmpegFromFfbinaries(tool string) error {
	var platformId = ""
	if runtime.GOOS == "windows" {
		switch runtime.GOARCH {
		case "386":
			platformId = "windows-32"
		default:
			platformId = "windows-64"
		}
	}
	if runtime.GOOS == "darwin" {
		platformId = "osx-64"
	}
	if runtime.GOOS == "linux" {
		switch runtime.GOARCH {
		case "386":
			platformId = "linux-32"
		case "amd64":
			platformId = "linux-64"
		case "arm":
			platformId = "linux-armhf"
		case "arm64":
			platformId = "linux-arm64"
		}
	}

	if platformId == "" {
		return errors.Errorf("Unknown architecture: %v/%v", runtime.GOOS, runtime.GOARCH)
	}

	resp, err := resty.New().R().Get("https://ffbinaries.com/api/v1/version/latest")
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return errors.Errorf("HTTP status code %d", resp.StatusCode())
	}

	url := gjson.Get(resp.String(), "bin."+platformId+"."+tool)

	err = downloadFile(url.String(), filepath.Join(common.BinDir, tool+".zip"))
	if err != nil {
		return err
	}

	err = archiver.Unarchive(filepath.Join(common.BinDir, tool+".zip"), common.BinDir)
	if err != nil {
		return err
	}

	_ = os.Remove(filepath.Join(common.BinDir, tool+".zip"))

	return nil
}

func copyFileLocal(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func downloadCaddy() error {
	var assetName string
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		assetName = "caddy_2.9.1_windows_amd64.zip"
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		assetName = "caddy_2.9.1_linux_amd64.tar.gz"
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		assetName = "caddy_2.9.1_linux_arm64.tar.gz"
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		assetName = "caddy_2.9.1_mac_amd64.zip"
	} else {
		return errors.Errorf("Caddy download not supported for %v/%v", runtime.GOOS, runtime.GOARCH)
	}

	url := "https://github.com/caddyserver/caddy/releases/download/v2.9.1/" + assetName
	log.Info("Downloading Caddy reverse proxy...")

	archivePath := filepath.Join(common.BinDir, "caddy-archive")
	if strings.HasSuffix(assetName, ".zip") {
		archivePath = archivePath + ".zip"
	} else {
		archivePath = archivePath + ".tar.gz"
	}

	err := downloadFile(url, archivePath)
	if err != nil {
		return err
	}

	tmpDir := filepath.Join(common.BinDir, "_caddy_extract")
	_ = os.RemoveAll(tmpDir)
	err = archiver.Unarchive(archivePath, tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = os.Remove(archivePath)
		return err
	}

	// Find caddy binary in extracted directory
	caddyName := "caddy"
	if runtime.GOOS == "windows" {
		caddyName = "caddy.exe"
	}
	found := false
	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == caddyName {
			destPath := filepath.Join(common.BinDir, caddyName)
			copyFileLocal(path, destPath)
			found = true
			return filepath.SkipDir
		}
		return nil
	})

	_ = os.RemoveAll(tmpDir)
	_ = os.Remove(archivePath)

	if !found {
		return errors.New("Caddy binary not found in archive")
	}
	return nil
}

func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.Errorf("HTTP status code %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
