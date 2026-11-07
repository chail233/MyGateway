package clients

import "golang.org/x/time/rate"

type Client struct {
	Name    string
	Limiter *rate.Limiter
}
