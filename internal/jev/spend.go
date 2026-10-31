package jev

type Spend struct {
	Requests     int
	InputTokens  int
	OutputTokens int
}

func (c *Client) Spent() Spend {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.spent
}

func (c *Client) countRequest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spent.Requests++
}

func (c *Client) countUsage(u Usage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spent.InputTokens += u.InputTokens
	c.spent.OutputTokens += u.OutputTokens
}
