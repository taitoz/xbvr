package tasks

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/xbapps/xbvr/pkg/common"
	"github.com/xbapps/xbvr/pkg/config"
)

var (
	caddyCmd     *exec.Cmd
	caddyStarted bool
	duckdnsStop  chan struct{}
)

func getCaddyPath() string {
	p := filepath.Join(common.BinDir, "caddy")
	if runtime.GOOS == "windows" {
		p = p + ".exe"
	}
	return p
}

func getCaddyfilePath() string {
	return filepath.Join(common.AppDir, "Caddyfile")
}

func writeCaddyfile() error {
	domain := config.Config.HTTPS.DuckDomain
	port := config.Config.Server.Port

	caddyfile := fmt.Sprintf("https://%s.duckdns.org {\n  reverse_proxy 127.0.0.1:%d\n}\n", domain, port)

	return os.WriteFile(getCaddyfilePath(), []byte(caddyfile), 0644)
}

func updateDuckDNS() error {
	domain := config.Config.HTTPS.DuckDomain
	token := config.Config.HTTPS.DuckToken

	if domain == "" || token == "" {
		return fmt.Errorf("DuckDNS domain or token not set")
	}

	url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&ip=", domain, token)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("DuckDNS update failed with status %d", resp.StatusCode)
	}

	log.Infof("DuckDNS updated for %s.duckdns.org", domain)
	return nil
}

func startDuckDNSUpdater() {
	duckdnsStop = make(chan struct{})
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Update immediately
	if err := updateDuckDNS(); err != nil {
		log.Warnf("DuckDNS initial update failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := updateDuckDNS(); err != nil {
				log.Warnf("DuckDNS update failed: %v", err)
			}
		case <-duckdnsStop:
			return
		}
	}
}

func StartCaddy() {
	if caddyStarted {
		return
	}

	caddyPath := getCaddyPath()
	if _, err := os.Stat(caddyPath); os.IsNotExist(err) {
		log.Info("Caddy not installed, downloading now...")
		if err := downloadCaddy(); err != nil {
			log.Errorf("Failed to download Caddy: %v", err)
			return
		}
	}

	if err := writeCaddyfile(); err != nil {
		log.Errorf("Failed to write Caddyfile: %v", err)
		return
	}

	// Start DuckDNS IP updater
	go startDuckDNSUpdater()

	// Start Caddy
	caddyCmd = exec.Command(caddyPath, "run", "--config", getCaddyfilePath())
	if runtime.GOOS == "windows" {
		caddyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}
	caddyCmd.Stdout = os.Stdout
	caddyCmd.Stderr = os.Stderr

	if err := caddyCmd.Start(); err != nil {
		log.Errorf("Failed to start Caddy: %v", err)
		return
	}

	caddyStarted = true
	domain := config.Config.HTTPS.DuckDomain
	port := config.Config.Server.Port
	log.Infof("Caddy started, HTTPS available at https://%s.duckdns.org (proxying to 127.0.0.1:%s)", domain, strconv.Itoa(port))
}

func StopCaddy() {
	if !caddyStarted {
		return
	}

	log.Info("Stopping Caddy")

	if duckdnsStop != nil {
		close(duckdnsStop)
		duckdnsStop = nil
	}

	if caddyCmd != nil && caddyCmd.Process != nil {
		caddyCmd.Process.Kill()
		caddyCmd.Wait()
	}

	caddyStarted = false
	caddyCmd = nil
}

func IsCaddyStarted() bool {
	return caddyStarted
}
