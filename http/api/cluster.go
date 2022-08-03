package api

type ClusterNodeConfig struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClusterNode struct {
	Address string `json:"address"`
	State   string `json:"state"`
}

type ClusterNodeFiles []string
