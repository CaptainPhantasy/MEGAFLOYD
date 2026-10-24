package client

func (c *Client) ServerHost() string {
	return c.network + "://" + c.addr
}
