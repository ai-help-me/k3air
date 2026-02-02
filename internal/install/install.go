package install

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"k3air/internal/config"
	"k3air/internal/sshclient"

	"gopkg.in/yaml.v3"
)

const (
	colorGreen = "\033[32m"
	colorReset = "\033[0m"
)

func green(s string) string {
	return colorGreen + s + colorReset
}

type Installer struct {
	cfg              config.Config
	assetsDir        string
	templateAssetsDir string
	assetManager     *AssetManager
	verbose          bool
}

func NewInstaller(cfg config.Config, assetsDir string, verbose bool) (*Installer, error) {
	am, err := NewAssetManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create asset manager: %w", err)
	}
	return &Installer{
		cfg:              cfg,
		assetsDir:        assetsDir,
		templateAssetsDir: assetsDir,
		assetManager:     am,
		verbose:          verbose,
	}, nil
}

func (i *Installer) Cleanup() error {
	return i.assetManager.Cleanup()
}

func (i *Installer) Apply() error {
	if len(i.cfg.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}
	primary := i.cfg.Servers[0]
	for idx, srv := range i.cfg.Servers {
		isPrimary := idx == 0
		slog.Info("install server", "node", srv.NodeName, "ip", srv.IP, "is primary", isPrimary)
		if err := i.installServer(srv, primary.IP, isPrimary); err != nil {
			return err
		}
	}
	for _, ag := range i.cfg.Agents {
		slog.Info("install agent", "node", ag.NodeName, "ip", ag.IP)
		if err := i.installAgent(ag, primary.IP); err != nil {
			return err
		}
	}
	if err := i.downloadKubeconfig(primary); err != nil {
		slog.Warn("failed to download kubeconfig", "error", err)
	}
	i.showClusterInfo(primary)
	i.printSuccessSummary(primary)
	return nil
}

func (i *Installer) installServer(node config.Node, primaryIP string, isPrimary bool) error {
	user := node.User
	if user == "" {
		user = "root"
	}
	c, err := sshclient.New(node.IP, node.Port, user, sshclient.Auth{Password: node.Password, KeyPath: node.KeyPath})
	if err != nil {
		return err
	}
	defer c.Close()

	slog.Info("SSH connected", "node", node.NodeName, "ip", node.IP)

	if isPrimary {
		slog.Info("initializing primary server", "node", node.NodeName)
	} else {
		slog.Info("joining control plane", "node", node.NodeName, "primary", primaryIP)
	}

	if err := i.prepareNode(c); err != nil {
		return err
	}
	if err := i.uploadAssets(c); err != nil {
		return err
	}

	// Generate uninstall script dynamically to use configured data-dir
	uninstallScript, err := i.uninstallScriptContent()
	if err != nil {
		return err
	}
	slog.Debug("uploading uninstall script")
	if err := c.UploadBytes([]byte(uninstallScript), "/usr/local/bin/k3s-uninstall.sh"); err != nil {
		return err
	}
	slog.Debug("setting uninstall script permissions")
	if err := runCmd(c, "chmod +x /usr/local/bin/k3s-uninstall.sh"); err != nil {
		return err
	}

	slog.Debug("generating systemd service file")
	svc := i.serverServiceContent(node, primaryIP, isPrimary)
	if err := c.UploadBytes([]byte(svc), "/etc/systemd/system/k3s.service"); err != nil {
		return err
	}

	slog.Debug("systemctl daemon-reload")
	if err := runCmd(c, "systemctl daemon-reload"); err != nil {
		return err
	}

	slog.Debug("systemctl enable k3s")
	if err := runCmd(c, "systemctl enable k3s"); err != nil {
		return err
	}

	slog.Info("starting k3s service")
	if err := runCmd(c, "systemctl restart k3s"); err != nil {
		return err
	}

	slog.Debug("waiting for service to start...", "seconds", 2)
	time.Sleep(2 * time.Second)

	slog.Debug("creating kubectl symlink")
	if err := runCmd(c, "cp /usr/local/bin/k3s /usr/local/bin/kubectl -f"); err != nil {
		return err
	}

	return nil
}

func (i *Installer) installAgent(node config.Node, primaryIP string) error {
	user := node.User
	if user == "" {
		user = "root"
	}
	c, err := sshclient.New(node.IP, node.Port, user, sshclient.Auth{Password: node.Password, KeyPath: node.KeyPath})
	if err != nil {
		return err
	}
	defer c.Close()

	slog.Info("SSH connected", "node", node.NodeName, "ip", node.IP)
	slog.Info("joining worker node", "node", node.NodeName, "server", primaryIP)

	if err := i.prepareNode(c); err != nil {
		return err
	}
	if err := i.uploadAssets(c); err != nil {
		return err
	}

	// Generate uninstall script dynamically to use configured data-dir
	agentUninstallScript, err := i.agentUninstallScriptContent()
	if err != nil {
		return err
	}
	slog.Debug("uploading uninstall script")
	if err := c.UploadBytes([]byte(agentUninstallScript), "/usr/local/bin/k3s-uninstall.sh"); err != nil {
		return err
	}
	slog.Debug("setting uninstall script permissions")
	if err := runCmd(c, "chmod +x /usr/local/bin/k3s-uninstall.sh"); err != nil {
		return err
	}

	slog.Debug("generating systemd service file")
	svc := i.agentServiceContent(node, primaryIP)
	if err := c.UploadBytes([]byte(svc), "/etc/systemd/system/k3s-agent.service"); err != nil {
		return err
	}

	slog.Debug("systemctl daemon-reload")
	if err := runCmd(c, "systemctl daemon-reload"); err != nil {
		return err
	}

	slog.Debug("systemctl enable k3s-agent")
	if err := runCmd(c, "systemctl enable k3s-agent"); err != nil {
		return err
	}

	slog.Info("starting k3s-agent service")
	if err := runCmd(c, "systemctl restart k3s-agent"); err != nil {
		return err
	}

	slog.Debug("waiting for service to start...", "seconds", 2)
	time.Sleep(2 * time.Second)

	return nil
}

func (i *Installer) prepareNode(c *sshclient.Client) error {
	slog.Info("preparing node environment", "node", c.Addr())

	slog.Debug("creating directory", "path", "/usr/local/bin")
	if err := runCmd(c, "mkdir -p /usr/local/bin"); err != nil {
		return err
	}

	imagesDir := filepath.Join(i.cfg.Cluster.DataDir, "agent", "images")
	slog.Debug("creating directory", "path", imagesDir)
	if err := runCmd(c, fmt.Sprintf("mkdir -p %s", imagesDir)); err != nil {
		return err
	}

	slog.Debug("creating directory", "path", "/etc/rancher/k3s")
	if err := runCmd(c, "mkdir -p /etc/rancher/k3s"); err != nil {
		return err
	}

	return nil
}

func (i *Installer) uploadAssets(c *sshclient.Client) error {
	slog.Info("uploading installation files", "node", c.Addr())

	// Resolve k3s binary (may be URL or local path)
	k3sPath, err := i.assetManager.ResolveAsset(i.cfg.Assets.K3sBinary, "k3s binary")
	if err != nil {
		return err
	}

	if fi, err := os.Stat(k3sPath); err == nil {
		slog.Info("uploading k3s binary", "size", formatBytes(fi.Size()), "node", c.Addr())
	}
	if err := c.Upload(k3sPath, "/usr/local/bin/k3s", true); err != nil {
		return err
	}

	slog.Debug("setting permissions", "path", "/usr/local/bin/k3s", "mode", "755")
	if err := runCmd(c, "chmod +x /usr/local/bin/k3s"); err != nil {
		return err
	}

	// Handle optional airgap images tarball
	if i.cfg.Assets.K3sAirgapTarball != "" {
		imgPath, err := i.assetManager.ResolveAsset(i.cfg.Assets.K3sAirgapTarball, "airgap images")
		if err != nil {
			// Only warn if images tarball is configured but not found
			slog.Warn("skipping images archive", "reason", err)
		} else {
			tarballPath := filepath.Join(i.cfg.Cluster.DataDir, "agent", "images", "k3s-airgap-images-amd64.tar.gz")
			if fi, err := os.Stat(imgPath); err == nil {
				slog.Info("uploading airgap images archive", "size", formatBytes(fi.Size()))
			}
			if err := c.Upload(imgPath, tarballPath, true); err != nil {
				return err
			}
		}
	} else {
		slog.Debug("no images archive configured")
	}

	if i.cfg.Cluster.Registries != "" {
		slog.Debug("uploading registries.yaml")
		if err := c.UploadBytes([]byte(i.cfg.Cluster.Registries), "/etc/rancher/k3s/registries.yaml"); err != nil {
			return err
		}
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (i *Installer) serverServiceContent(node config.Node, primaryIP string, isPrimary bool) string {
	cluster := i.cfg.Cluster
	var args []string
	if isPrimary {
		args = append(args, "server", "--cluster-init")
	} else {
		args = append(args, "server", "--server", fmt.Sprintf("https://%s:6443", primaryIP))
	}
	if cluster.FlannelBackend != "" {
		args = append(args, "--flannel-backend", cluster.FlannelBackend)
	}
	if cluster.ClusterCidr != "" {
		args = append(args, "--cluster-cidr", cluster.ClusterCidr)
	}
	if cluster.ServiceCidr != "" {
		args = append(args, "--service-cidr", cluster.ServiceCidr)
	}
	if cluster.DataDir != "" {
		args = append(args, "--data-dir", cluster.DataDir)
	}
	if node.NodeName != "" {
		args = append(args, "--node-name", node.NodeName)
	}
	if cluster.EmbeddedRegistry {
		args = append(args, "--embedded-registry")
	}
	for _, s := range cluster.TLSSAN {
		if s != "" {
			args = append(args, "--tls-san", s)
		}
	}
	for _, d := range cluster.Disable {
		if d != "" {
			args = append(args, "--disable", d)
		}
	}
	for _, l := range node.Labels {
		if l != "" {
			args = append(args, "--node-label", l)
		}
	}
	cmd := "/usr/local/bin/k3s " + strings.Join(args, " ") + " --token " + cluster.Token
	return unitService("k3s", cmd)
}

func (i *Installer) agentServiceContent(node config.Node, primaryIP string) string {
	cluster := i.cfg.Cluster
	var args []string
	args = append(args, "agent", "--server", fmt.Sprintf("https://%s:6443", primaryIP))
	if cluster.DataDir != "" {
		args = append(args, "--data-dir", cluster.DataDir)
	}
	if node.NodeName != "" {
		args = append(args, "--node-name", node.NodeName)
	}
	for _, l := range node.Labels {
		if l != "" {
			args = append(args, "--node-label", l)
		}
	}
	args = append(args, "--token", cluster.Token)
	cmd := "/usr/local/bin/k3s " + strings.Join(args, " ")
	return unitService("k3s-agent", cmd)
}

func (i *Installer) showClusterInfo(master config.Node) {
	user := master.User
	if user == "" {
		user = "root"
	}
	c, err := sshclient.New(master.IP, master.Port, user, sshclient.Auth{Password: master.Password, KeyPath: master.KeyPath})
	if err != nil {
		slog.Error("failed to connect to master node", "error", err)
		return
	}
	defer c.Close()
	if err := runCmd(c, "kubectl get nodes"); err != nil {
		slog.Error("failed to get nodes", "error", err)
		return
	}
	nodes, _, _ := c.Run("kubectl get nodes")
	fmt.Println(green("Cluster Nodes:"))
	fmt.Println(nodes)
}

func (i *Installer) printSuccessSummary(master config.Node) {
	fmt.Println()
	fmt.Println(green("=" + strings.Repeat("=", 50)))
	fmt.Println(green("✓ Installation completed successfully!"))
	fmt.Println(green("=" + strings.Repeat("=", 50)))
	fmt.Println()
	fmt.Println("To access your cluster, set the KUBECONFIG environment variable:")
	fmt.Println(green("  export KUBECONFIG=$(pwd)/kubeconfig"))
	fmt.Println()
	fmt.Println("Then run kubectl commands:")
	fmt.Println(green("  kubectl get nodes"))
	fmt.Println(green("  kubectl get pods -A"))
	fmt.Println()
	fmt.Printf("API Server: %s:6443\n", master.IP)
	fmt.Println()
}

func unitService(name, exec string) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=" + name + "\n")
	b.WriteString("After=network.target\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=notify\n")
	b.WriteString("ExecStart=" + exec + "\n")
	b.WriteString("Restart=always\n")
	b.WriteString("LimitNOFILE=1048576\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

func runCmd(c *sshclient.Client, cmd string) error {
	stdout, stderr, err := c.Run(cmd)
	if err != nil {
		return fmt.Errorf("cmd failed: %s\nstdout:\n%s\nstderr:\n%s\nerr: %v", cmd, stdout, stderr, err)
	}
	return nil
}

func (i *Installer) downloadKubeconfig(master config.Node) error {
	slog.Info("downloading kubeconfig", "from", master.IP)

	user := master.User
	if user == "" {
		user = "root"
	}
	c, err := sshclient.New(master.IP, master.Port, user, sshclient.Auth{Password: master.Password, KeyPath: master.KeyPath})
	if err != nil {
		return err
	}
	defer c.Close()

	// Kubeconfig path on remote server
	remoteKubeconfig := filepath.Join(i.cfg.Cluster.DataDir, "server", "cred", "k3s.yaml")
	slog.Debug("trying kubeconfig path", "path", remoteKubeconfig)

	// Try default location if data-dir path doesn't work
	content, err := c.DownloadBytes(remoteKubeconfig)
	if err != nil {
		slog.Debug("using fallback path", "path", "/etc/rancher/k3s/k3s.yaml")
		// Fallback to default k3s location
		content, err = c.DownloadBytes("/etc/rancher/k3s/k3s.yaml")
		if err != nil {
			return fmt.Errorf("failed to download kubeconfig: %w", err)
		}
	}

	// Parse and modify kubeconfig using YAML parsing
	modified, replaced, err := replaceKubeconfigServer(content, master.IP)
	if err != nil {
		return fmt.Errorf("failed to modify kubeconfig: %w", err)
	}
	if replaced {
		slog.Info("replaced 127.0.0.1 with server IP in kubeconfig", "ip", master.IP)
	}

	// Write to local file
	localPath := "kubeconfig"
	slog.Debug("saving kubeconfig", "path", localPath)
	if err := os.WriteFile(localPath, modified, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	slog.Info("kubeconfig saved", "path", localPath)
	fmt.Println(green("✓ Kubeconfig written to: " + localPath))
	return nil
}

// replaceKubeconfigServer parses the kubeconfig YAML and replaces the server URL
func replaceKubeconfigServer(data []byte, serverIP string) ([]byte, bool, error) {
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, false, err
	}

	replaced := false

	// Navigate to clusters[0].cluster.server
	if clusters, ok := config["clusters"].([]interface{}); ok && len(clusters) > 0 {
		if firstCluster, ok := clusters[0].(map[string]interface{}); ok {
			if clusterData, ok := firstCluster["cluster"].(map[string]interface{}); ok {
				if serverURL, ok := clusterData["server"].(string); ok {
					// Check if server URL contains 127.0.0.1
					newURL := strings.ReplaceAll(serverURL, "127.0.0.1", serverIP)
					if newURL != serverURL {
						clusterData["server"] = newURL
						replaced = true
					}
				}
			}
		}
	}

	// Marshal back to YAML
	modified, err := yaml.Marshal(config)
	return modified, replaced, err
}

// uninstallScriptContent generates the uninstall script content using configured data-dir
func (i *Installer) uninstallScriptContent() (string, error) {
	dataDir := i.cfg.Cluster.DataDir
	if dataDir == "" {
		dataDir = "/var/lib/rancher/k3s"
	}

	tmpl, err := template.New("uninstall").Parse(uninstallTmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse uninstall template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		DataDir string
		IsAgent bool
	}{
		DataDir: dataDir,
		IsAgent: false,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute uninstall template: %w", err)
	}

	return buf.String(), nil
}

// agentUninstallScriptContent generates the uninstall script content for agent nodes
func (i *Installer) agentUninstallScriptContent() (string, error) {
	dataDir := i.cfg.Cluster.DataDir
	if dataDir == "" {
		dataDir = "/var/lib/rancher/k3s"
	}

	tmpl, err := template.New("uninstall").Parse(uninstallTmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse uninstall template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		DataDir string
		IsAgent bool
	}{
		DataDir: dataDir,
		IsAgent: true,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute uninstall template: %w", err)
	}

	return buf.String(), nil
}
