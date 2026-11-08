package clients

type Client struct {
	Name     string
	Upstream []Upstream
}

type APIKey struct {
	Key      string     `json:"key"`
	Name     string     `json:"name"`
	Upstream []Upstream `json:"upstream"`
}

type Upstream struct {
	BaseURL string `json:"base_url"`
	Key     string `json:"key"`
}
