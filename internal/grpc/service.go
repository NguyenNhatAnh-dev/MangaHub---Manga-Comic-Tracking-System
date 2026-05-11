package grpcsvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/mangahub/mangahub/internal/manga"
	"github.com/mangahub/mangahub/pkg/models"
	"github.com/mangahub/mangahub/pkg/protocol"
	"google.golang.org/grpc"
)

const ServiceName = "mangahub.v1.MangaService"

type GetMangaRequest struct {
	ID string `json:"id"`
}

type MangaMessage struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url"`
}

type SearchRequest struct {
	Query  string `json:"query"`
	Genre  string `json:"genre"`
	Status string `json:"status"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type SearchResponse struct {
	Results []*MangaMessage `json:"results"`
	Count   int             `json:"count"`
}

type ProgressRequest struct {
	UserID  string `json:"user_id"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
}

type ProgressResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type MangaServiceServer interface {
	GetManga(ctx context.Context, req *GetMangaRequest) (*MangaMessage, error)
	SearchManga(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
	UpdateProgress(ctx context.Context, req *ProgressRequest) (*ProgressResponse, error)
}

func _GetManga_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMangaRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MangaServiceServer).GetManga(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/GetManga"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MangaServiceServer).GetManga(ctx, req.(*GetMangaRequest))
	})
}

func _SearchManga_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SearchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MangaServiceServer).SearchManga(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/SearchManga"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MangaServiceServer).SearchManga(ctx, req.(*SearchRequest))
	})
}

func _UpdateProgress_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ProgressRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MangaServiceServer).UpdateProgress(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/UpdateProgress"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MangaServiceServer).UpdateProgress(ctx, req.(*ProgressRequest))
	})
}

var serviceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*MangaServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "GetManga", Handler: _GetManga_Handler},
		{MethodName: "SearchManga", Handler: _SearchManga_Handler},
		{MethodName: "UpdateProgress", Handler: _UpdateProgress_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "manga.proto",
}

func RegisterMangaServiceServer(s *grpc.Server, srv MangaServiceServer) {
	s.RegisterService(&serviceDesc, srv)
}

type Server struct {
	addr      string
	mangaRepo *manga.Repository
	broker    *protocol.Broker
	grpcSrv   *grpc.Server
}

func NewServer(addr string, db *sql.DB) *Server {
	return &Server{
		addr:      addr,
		mangaRepo: manga.NewRepository(db),
		broker:    protocol.Default(),
	}
}

func (s *Server) Start() error {
	RegisterCodec()
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", s.addr, err)
	}
	s.grpcSrv = grpc.NewServer()
	RegisterMangaServiceServer(s.grpcSrv, s)
	log.Printf("[gRPC] Listening on %s", s.addr)
	return s.grpcSrv.Serve(ln)
}

func (s *Server) Stop() {
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
}

func (s *Server) GetManga(ctx context.Context, req *GetMangaRequest) (*MangaMessage, error) {
	if req.ID == "" {
		return nil, errors.New("id is required")
	}
	m, err := s.mangaRepo.GetByID(req.ID)
	if err != nil {
		return nil, err
	}
	return mangaToMessage(m), nil
}

func (s *Server) SearchManga(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	results, err := s.mangaRepo.Search(req.Query, req.Genre, req.Status, req.Limit, req.Offset)
	if err != nil {
		return nil, err
	}
	out := &SearchResponse{Count: len(results)}
	for _, m := range results {
		out.Results = append(out.Results, mangaToMessage(m))
	}
	return out, nil
}

func (s *Server) UpdateProgress(ctx context.Context, req *ProgressRequest) (*ProgressResponse, error) {
	if req.UserID == "" || req.MangaID == "" {
		return &ProgressResponse{Success: false, Message: "user_id and manga_id required"}, nil
	}
	if err := s.mangaRepo.UpdateProgress(req.UserID, req.MangaID, req.Chapter); err != nil {
		return &ProgressResponse{Success: false, Message: err.Error()}, nil
	}
	s.broker.PublishProgress(models.ProgressUpdate{
		UserID:    req.UserID,
		MangaID:   req.MangaID,
		Chapter:   req.Chapter,
		Timestamp: time.Now().Unix(),
	})
	return &ProgressResponse{Success: true, Message: "progress updated"}, nil
}

func mangaToMessage(m *models.Manga) *MangaMessage {
	return &MangaMessage{
		ID:            m.ID,
		Title:         m.Title,
		Author:        m.Author,
		Genres:        m.Genres,
		Status:        m.Status,
		TotalChapters: m.TotalChapters,
		Description:   m.Description,
		CoverURL:      m.CoverURL,
	}
}
