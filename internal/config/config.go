package config

import (
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
	return c, nil
}
