package api

type ClusterNodeConfig struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClusterNode struct {
	Address     string  `json:"address"`
	ID          string  `json:"id"`
	LastContact int64   `json:"last_contact"` // unix timestamp
	Latency     float64 `json:"latency_ms"`   // milliseconds
	State       string  `json:"state"`
	CPU         float64 `json:"cpu_used"` // percent
	Mem         float64 `json:"mem_used"` // percent
}

type ClusterNodeFiles struct {
	LastUpdate int64               `json:"last_update"` // unix timestamp
	Files      map[string][]string `json:"files"`
}

type ClusterServer struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Voter   bool   `json:"voter"`
	Leader  bool   `json:"leader"`
}

type ClusterStats struct {
	State       string  `json:"state"`
	LastContact float64 `json:"last_contact_ms"`
	NumPeers    uint64  `json:"num_peers"`
}

type ClusterAbout struct {
	ID                string          `json:"id"`
	Address           string          `json:"address"`
	ClusterAPIAddress string          `json:"cluster_api_address"`
	CoreAPIAddress    string          `json:"core_api_address"`
	Nodes             []ClusterServer `json:"nodes"`
	Stats             ClusterStats    `json:"stats"`
}
