package httpapi

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mangahub/mangahub/internal/auth"
	"github.com/mangahub/mangahub/internal/manga"
	"github.com/mangahub/mangahub/internal/user"
	"github.com/mangahub/mangahub/pkg/models"
	"github.com/mangahub/mangahub/pkg/protocol"
)

type Server struct {
	router    *gin.Engine
	authSvc   *auth.Service
	userRepo  *user.Repository
	mangaRepo *manga.Repository
	broker    *protocol.Broker
}

func NewServer(db *sql.DB, jwtSecret string) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(loggingMiddleware())

	s := &Server{
		router:    r,
		authSvc:   auth.NewService(jwtSecret),
		userRepo:  user.NewRepository(db),
		mangaRepo: manga.NewRepository(db),
		broker:    protocol.Default(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		gin.DefaultWriter.Write([]byte(
			"[HTTP] " + c.Request.Method + " " + c.Request.URL.Path + " -> " +
				strconv.Itoa(c.Writer.Status()) + " (" + time.Since(start).String() + ")\n",
		))
	}
}

func (s *Server) routes() {
	r := s.router
	r.GET("/health", s.health)

	authGrp := r.Group("/auth")
	{
		authGrp.POST("/register", s.register)
		authGrp.POST("/login", s.login)
	}

	mangaGrp := r.Group("/manga")
	{
		mangaGrp.GET("", s.searchManga)
		mangaGrp.GET("/:id", s.getManga)
	}

	users := r.Group("/users")
	users.Use(s.authSvc.Middleware())
	{
		users.GET("/me", s.me)
		users.GET("/library", s.getLibrary)
		users.POST("/library", s.addToLibrary)
		users.DELETE("/library/:manga_id", s.removeFromLibrary)
		users.PUT("/progress", s.updateProgress)
		users.PUT("/rating", s.updateRating)
	}

	admin := r.Group("/admin")
	{
		admin.POST("/manga", s.adminAddManga)
		admin.POST("/notify", s.adminNotify)
	}
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Unix()})
}

func (s *Server) register(c *gin.Context) {
	var req models.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username, email and password are required"})
		return
	}
	hash, err := s.authSvc.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := auth.GenerateID("usr")
	u, err := s.userRepo.Create(id, req.Username, req.Email, hash)
	if err != nil {
		if err == user.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
			return
		}
		if err == user.ErrInvalidEmail {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"user_id":  u.ID,
		"username": u.Username,
		"email":    u.Email,
	})
}

func (s *Server) login(c *gin.Context) {
	var req models.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	identifier := req.Username
	if identifier == "" {
		identifier = req.Email
	}
	u, err := s.userRepo.GetByUsername(identifier)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !s.authSvc.VerifyPassword(u.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	token, err := s.authSvc.GenerateToken(u.ID, u.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.AuthResponse{
		Token:    token,
		UserID:   u.ID,
		Username: u.Username,
	})
}

func (s *Server) me(c *gin.Context) {
	uid := c.GetString("user_id")
	u, err := s.userRepo.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, u)
}

func (s *Server) searchManga(c *gin.Context) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	results, err := s.mangaRepo.Search(q, genre, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"count":   len(results),
		"results": results,
	})
}

func (s *Server) getManga(c *gin.Context) {
	id := c.Param("id")
	m, err := s.mangaRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	resp := gin.H{"manga": m}
	if uid, ok := c.Get("user_id"); ok {
		if p, err := s.mangaRepo.GetProgress(uid.(string), id); err == nil {
			resp["progress"] = p
		}
	}
	c.JSON(http.StatusOK, resp)
}

type addLibraryRequest struct {
	MangaID string `json:"manga_id"`
	Status  string `json:"status"`
	Chapter int    `json:"chapter"`
}

func (s *Server) addToLibrary(c *gin.Context) {
	var req addLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.MangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id is required"})
		return
	}
	if _, err := s.mangaRepo.GetByID(req.MangaID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	uid := c.GetString("user_id")
	if err := s.mangaRepo.AddToLibrary(uid, req.MangaID, req.Status, req.Chapter); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "added"})
}

func (s *Server) removeFromLibrary(c *gin.Context) {
	uid := c.GetString("user_id")
	mangaID := c.Param("manga_id")
	if err := s.mangaRepo.RemoveFromLibrary(uid, mangaID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (s *Server) getLibrary(c *gin.Context) {
	uid := c.GetString("user_id")
	status := c.Query("status")
	entries, err := s.mangaRepo.GetLibrary(uid, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(entries), "library": entries})
}

type updateProgressRequest struct {
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
}

func (s *Server) updateProgress(c *gin.Context) {
	var req updateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.MangaID == "" || req.Chapter < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id and non-negative chapter required"})
		return
	}
	m, err := s.mangaRepo.GetByID(req.MangaID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	if m.TotalChapters > 0 && req.Chapter > m.TotalChapters {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter exceeds total chapters"})
		return
	}
	uid := c.GetString("user_id")
	if err := s.mangaRepo.UpdateProgress(uid, req.MangaID, req.Chapter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.broker.PublishProgress(models.ProgressUpdate{
		UserID:    uid,
		MangaID:   req.MangaID,
		Chapter:   req.Chapter,
		Timestamp: time.Now().Unix(),
	})

	c.JSON(http.StatusOK, gin.H{"status": "updated", "chapter": req.Chapter})
}

type ratingRequest struct {
	MangaID string `json:"manga_id"`
	Rating  int    `json:"rating"`
}

func (s *Server) updateRating(c *gin.Context) {
	var req ratingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Rating < 0 || req.Rating > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be 0-10"})
		return
	}
	uid := c.GetString("user_id")
	if err := s.mangaRepo.UpdateRating(uid, req.MangaID, req.Rating); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) adminAddManga(c *gin.Context) {
	var m models.Manga
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if m.ID == "" || m.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and title are required"})
		return
	}
	if err := s.mangaRepo.Upsert(&m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "created", "id": m.ID})
}

type notifyRequest struct {
	MangaID string `json:"manga_id"`
	Message string `json:"message"`
}

func (s *Server) adminNotify(c *gin.Context) {
	var req notifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}
	s.broker.PublishNotification(models.Notification{
		Type:      "chapter_release",
		MangaID:   req.MangaID,
		Message:   req.Message,
		Timestamp: time.Now().Unix(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
}
