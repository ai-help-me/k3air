# k3air

k3air 是一个使用 Go 编写在离线环境下安装高可用 k3s 集群的命令行工具，它不依赖任何外部服务网络，所有的组件都在本地运行。k3air 支持一个 yaml 格式的配置文件，用户可以在配置文件中定义 k3s 集群的节点（节点数量没有限制，可以是一个，也可以是多个），网络，启动参数等信息，所有依赖都打包成离线文件，然后通过 k3air 命令来 k3s 集群。

1. 生成默认配置
```bash
k3air init
```
2. 编辑配置文件
3. 开始部署
```bash
# 1m15s 内拉起一套三节点 k3s 集群
k3air apply -f init.yaml
```
[![asciicast](https://asciinema.org/a/UPheMWJ2lBPFfrrx.svg)](https://asciinema.org/a/UPheMWJ2lBPFfrrx)
配置文件示例：
```yaml
# =============================================================================
# k3air 集群配置文件
# =============================================================================
# 此文件用于定义 k3s 高可用集群的部署配置
# 修改完成后运行: k3air apply -f init.yaml
# =============================================================================

# -----------------------------------------------------------------------------
# 集群配置 (cluster)
# -----------------------------------------------------------------------------
cluster:
    # Flannel 后端网络类型
    # 可选值: vxlan (默认), host-gw, none, wireguard
    # vxlan: 适用于有 overlay 网络的场景，性能略低但兼容性好
    # host-gw: 主机网关模式，性能更好但要求节点在同一二层网络
    # none: 禁用默认 CNI，使用自定义网络插件
    flannel-backend: vxlan

    # Pod 网络地址段 (Cluster CIDR)
    # 用于分配 Pod IP 地址的范围
    # 默认值: 10.42.0.0/16
    # 可选: 不填则使用默认值
    cluster-cidr: 10.42.0.0/16

    # Service 网络地址段 (Service CIDR)
    # 用于分配 ClusterIP Service 的 IP 地址范围
    # 默认值: 10.43.0.0/16
    # 可选: 不填则使用默认值
    service-cidr: 10.43.0.0/16

    # 集群认证令牌
    # 用于服务器和代理节点之间通信的共享密钥
    # 默认值: 无，必须指定
    # 建议: 使用随机生成的字符串，如: openssl rand -hex 16
    token: "k3air-token"

    # TLS 额外主题备用名称 (Subject Alternative Names)
    # 用于 API Server 证书的额外域名或 IP
    # 适用场景: 使用负载均衡器或自定义域名访问集群时
    # 示例: ["192.168.1.100", "k3s.example.com", "lb.internal"]
    # 可选: 不填则不添加额外 SAN
    tls-san: []

    # 禁用的组件列表
    # 禁用不需要的 k3s 内置组件以节省资源
    # 常用选项:
    #   - traefik: 默认 Ingress 控制器
    #   - metrics-server: 集群指标监控
    #   - servicelb: Kubernetes Service 负载均衡器
    # 可选: 不填则启用所有组件
    disable: []

    # 数据存储目录
    # k3s 存储数据、证书和数据库的路径
    # 默认值: /var/lib/rancher/k3s
    # 可选: 不填则使用默认值
    data-dir: /var/lib/rancher/k3s

    # 是否启用嵌入式容器镜像仓库
    # true: 在集群内部启动一个私有镜像仓库，用于离线环境
    # false: 使用默认配置
    # 默认值: false
    # 可选: 不填则使用默认值 false
    embedded-registry: true

    # 私有镜像仓库配置 (registries.yaml)
    # 用于配置 Docker/Containerd 的私有镜像仓库
    # 详细格式见: https://docs.k3s.io/installation/private-registry
    # 示例:
    # registries: |
    #   mirrors:
    #     docker.io:
    #       endpoint:
    #         - "https://registry-1.docker.io"
    #     my-registry.local:
    #       endpoint:
    #         - "http://my-registry.local"
    #   configs:
    #     my-registry.local:
    #       auth:
    #         username: admin
    #         password: password
    # 可选: 不填则不配置私有仓库
    #registries: ""

# -----------------------------------------------------------------------------
# 资源文件配置 (assets)
# -----------------------------------------------------------------------------
assets:
    # K3s 二进制文件路径
    # 支持三种格式:
    #   1. URL: 自动下载，如 https://github.com/k3s-io/k3s/releases/download/v1.28.5+k3s1/k3s
    #   2. 相对路径: 相对于 k3air 运行目录，如 k3s 或 ./k3s
    #   3. 绝对路径: 如 /opt/k3s-binary/k3s
    # 默认值: k3s
    # 可选: 不填则使用默认值 k3s
    k3s-binary: ./k3s

    # K3s 离线镜像压缩包路径
    # 包含所有 k3s 所需容器镜像的 tar.gz 文件
    # 支持三种格式 (同 k3s-binary):
    #   1. URL: 如 https://github.com/k3s-io/k3s/releases/download/v1.28.5+k3s1/k3s-airgap-images-amd64.tar.gz
    #   2. 相对路径: 如 k3s-airgap-images-amd64.tar.gz
    #   3. 绝对路径: 如 /opt/images/k3s-airgap-images-amd64.tar.gz
    # 默认值: k3s-airgap-images-amd64.tar.gz
    # 可选: 不填则使用默认值
    k3s-airgap-tarball: ./k3s-airgap-images-amd64.tar.gz

# -----------------------------------------------------------------------------
# 控制平面节点配置 (servers)
# -----------------------------------------------------------------------------
# 第一个服务器将作为主节点 (Primary Server)，使用 --cluster-init 初始化
# 后续服务器将作为从节点加入主节点，形成高可用集群
# 至少需要 1 个服务器节点，建议奇数个节点（3/5/7）用于高可用
servers:
    - node_name: k3s-server-0
      ip: 10.0.0.1
      # SSH 端口
      # 默认值: 22
      # 可选: 不填则使用默认值
      port: 22
      # SSH 登录用户
      # 默认值: root
      # 可选: 不填则使用默认值
      user: root
      # SSH 密码认证
      # 与 key_path 二选一，优先使用 key_path
      # 可选: 不填则必须指定 key_path
      password: "123456"
      # SSH 私钥路径
      # 与 password 二选一，优先使用 key_path
      # 示例: /root/.ssh/id_rsa
      # 可选: 不填则必须指定 password
      #key_path: ""
      # 节点标签 (Node Labels)
      # 用于给节点打标签，用于 Pod 调度约束
      # 示例: ["disk=ssd", "zone=us-west-1", "node-role.kubernetes.io/worker=true"]
      # 可选: 不填则不添加标签
#     labels: []

#   - node_name: k3s-server-1
#     ip: 10.0.0.2
#     port: 22
#     user: root
#     password: "123456"
#     key_path: ""
#     labels: []

#   - node_name: k3s-server-2
#     ip: 10.0.0.3
#     port: 22
#     user: root
#     password: "123456"
#     key_path: ""
#     labels: []

# -----------------------------------------------------------------------------
# 工作节点配置 (agents)
# -----------------------------------------------------------------------------
# Agent 节点运行工作负载，不参与控制平面
# 可选: 不填则仅部署控制平面节点
#agents:
#    - node_name: k3s-agent-0
#      ip: 10.0.0.4
#      port: 22
#      user: root
#      password: "123456"
#      key_path: ""
#     # 工作节点通常设置一些标签用于调度
#     labels:
#       - disk=ssd
```
