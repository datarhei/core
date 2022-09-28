package api

type ClusterNodeConfig struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClusterNode struct {
	Address    string `json:"address"`
	ID         string `json:"id"`
	LastUpdate int64  `json:"last_update"`
	State      string `json:"state"`
}

type ClusterNodeFiles map[string][]string
