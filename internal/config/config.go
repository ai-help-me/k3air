package config

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

type AssetSource struct {
	K3sBinary       string `yaml:"k3s-binary"`
	K3sAirgapTarball string `yaml:"k3s-airgap-tarball"`
}

type Cluster struct {
	FlannelBackend   string   `yaml:"flannel-backend"`
	ClusterCidr      string   `yaml:"cluster-cidr"`
	ServiceCidr      string   `yaml:"service-cidr"`
	Token            string   `yaml:"token"`
	TLSSAN           []string `yaml:"tls-san"`
	Disable          []string `yaml:"disable"`
	DataDir          string   `yaml:"data-dir"`
	EmbeddedRegistry bool     `yaml:"embedded-registry"`
	Registries       string   `yaml:"registries"`
}

type Node struct {
	NodeName string   `yaml:"node_name"`
	IP       string   `yaml:"ip"`
	Port     int      `yaml:"port"`
	User     string   `yaml:"user"`
	Password string   `yaml:"password"`
	KeyPath  string   `yaml:"key_path"`
	Labels   []string `yaml:"labels"`
}

type Config struct {
	Cluster Cluster     `yaml:"cluster"`
	Assets  AssetSource `yaml:"assets"`
	Servers []Node      `yaml:"servers"`
	Agents  []Node      `yaml:"agents"`
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Cluster.ClusterCidr == "" {
		c.Cluster.ClusterCidr = "10.42.0.0/16"
	}
	if c.Cluster.ServiceCidr == "" {
		c.Cluster.ServiceCidr = "10.43.0.0/16"
	}
	if c.Cluster.DataDir == "" {
		c.Cluster.DataDir = "/var/lib/rancher/k3s"
	}
	if c.Cluster.FlannelBackend == "" {
		c.Cluster.FlannelBackend = "vxlan"
	}
	if c.Assets.K3sBinary == "" {
		c.Assets.K3sBinary = "k3s"
	}
	if c.Assets.K3sAirgapTarball == "" {
		c.Assets.K3sAirgapTarball = "k3s-airgap-images-amd64.tar.gz"
	}
	// Set default port to 22 if not specified
	for i := range c.Servers {
		if c.Servers[i].Port == 0 {
			c.Servers[i].Port = 22
		}
	}
	for i := range c.Agents {
		if c.Agents[i].Port == 0 {
			c.Agents[i].Port = 22
		}
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("config validation failed: %w", err)
	}
	return c, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate CIDR formats
	clusterCIDR, err := parseAndValidateCIDR(c.Cluster.ClusterCidr, "cluster-cidr")
	if err != nil {
		return err
	}
	serviceCIDR, err := parseAndValidateCIDR(c.Cluster.ServiceCidr, "service-cidr")
	if err != nil {
		return err
	}

	// Check if CIDRs are identical
	if cidrsEqual(clusterCIDR, serviceCIDR) {
		return fmt.Errorf("cluster-cidr and service-cidr cannot be the same: %s", c.Cluster.ClusterCidr)
	}

	// Check if CIDRs overlap
	if cidrsOverlap(clusterCIDR, serviceCIDR) {
		return fmt.Errorf("cluster-cidr (%s) and service-cidr (%s) overlap", c.Cluster.ClusterCidr, c.Cluster.ServiceCidr)
	}

	// Validate node IPs
	for _, node := range c.Servers {
		if err := validateNodeIP(node); err != nil {
			return fmt.Errorf("server %s: %w", node.NodeName, err)
		}
	}
	for _, node := range c.Agents {
		if err := validateNodeIP(node); err != nil {
			return fmt.Errorf("agent %s: %w", node.NodeName, err)
		}
	}

	return nil
}

// parseAndValidateCIDR parses and validates a CIDR string
func parseAndValidateCIDR(cidrStr, fieldName string) (*net.IPNet, error) {
	_, cidr, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %s (error: %w)", fieldName, cidrStr, err)
	}
	return cidr, nil
}

// cidrsEqual checks if two CIDRs are exactly the same
func cidrsEqual(a, b *net.IPNet) bool {
	return a.IP.Equal(b.IP) && bytesEqual(a.Mask, b.Mask)
}

// bytesEqual compares two byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cidrsOverlap checks if two CIDR ranges overlap
func cidrsOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// validateNodeIP validates a node's IP address
func validateNodeIP(node Node) error {
	if node.IP == "" {
		return fmt.Errorf("ip address is empty")
	}
	ip := net.ParseIP(node.IP)
	if ip == nil {
		return fmt.Errorf("invalid ip address: %s", node.IP)
	}
	return nil
}
