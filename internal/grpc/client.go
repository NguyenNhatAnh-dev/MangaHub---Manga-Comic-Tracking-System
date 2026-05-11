package grpcsvc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn *grpc.ClientConn
}

func NewClient(addr string) (*Client, error) {
	RegisterCodec()
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{})),
	)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GetManga(ctx context.Context, id string) (*MangaMessage, error) {
	out := new(MangaMessage)
	err := c.conn.Invoke(ctx, "/"+ServiceName+"/GetManga", &GetMangaRequest{ID: id}, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) SearchManga(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	out := new(SearchResponse)
	err := c.conn.Invoke(ctx, "/"+ServiceName+"/SearchManga", req, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdateProgress(ctx context.Context, userID, mangaID string, chapter int) (*ProgressResponse, error) {
	out := new(ProgressResponse)
	err := c.conn.Invoke(ctx, "/"+ServiceName+"/UpdateProgress",
		&ProgressRequest{UserID: userID, MangaID: mangaID, Chapter: chapter}, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func DefaultTimeout() time.Duration {
	return 5 * time.Second
}
