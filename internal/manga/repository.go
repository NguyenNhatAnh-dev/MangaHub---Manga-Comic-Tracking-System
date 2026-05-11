package manga

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mangahub/mangahub/pkg/models"
)

var ErrMangaNotFound = errors.New("manga not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(m *models.Manga) error {
	genresJSON, err := json.Marshal(m.Genres)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO manga (id, title, author, genres, status, total_chapters, description, cover_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title,
			author=excluded.author,
			genres=excluded.genres,
			status=excluded.status,
			total_chapters=excluded.total_chapters,
			description=excluded.description,
			cover_url=excluded.cover_url
	`, m.ID, m.Title, m.Author, string(genresJSON), m.Status, m.TotalChapters, m.Description, m.CoverURL)
	return err
}

func (r *Repository) GetByID(id string) (*models.Manga, error) {
	m := &models.Manga{}
	var genresJSON string
	err := r.db.QueryRow(`
		SELECT id, title, author, genres, status, total_chapters, description, cover_url
		FROM manga WHERE id = ?`, id,
	).Scan(&m.ID, &m.Title, &m.Author, &genresJSON, &m.Status, &m.TotalChapters, &m.Description, &m.CoverURL)
	if err == sql.ErrNoRows {
		return nil, ErrMangaNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(genresJSON), &m.Genres)
	return m, nil
}

func (r *Repository) Search(query, genre, status string, limit, offset int) ([]*models.Manga, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	conditions := []string{}
	args := []interface{}{}

	if query != "" {
		conditions = append(conditions, "(LOWER(title) LIKE ? OR LOWER(author) LIKE ?)")
		like := "%" + strings.ToLower(query) + "%"
		args = append(args, like, like)
	}
	if genre != "" {
		conditions = append(conditions, "LOWER(genres) LIKE ?")
		args = append(args, "%"+strings.ToLower(genre)+"%")
	}
	if status != "" {
		conditions = append(conditions, "LOWER(status) = ?")
		args = append(args, strings.ToLower(status))
	}

	sqlQuery := "SELECT id, title, author, genres, status, total_chapters, description, cover_url FROM manga"
	if len(conditions) > 0 {
		sqlQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	sqlQuery += " ORDER BY title LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []*models.Manga{}
	for rows.Next() {
		m := &models.Manga{}
		var genresJSON string
		if err := rows.Scan(&m.ID, &m.Title, &m.Author, &genresJSON, &m.Status, &m.TotalChapters, &m.Description, &m.CoverURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genresJSON), &m.Genres)
		results = append(results, m)
	}
	return results, rows.Err()
}

func (r *Repository) AddToLibrary(userID, mangaID, status string, chapter int) error {
	if status == "" {
		status = "plan-to-read"
	}
	_, err := r.db.Exec(`
		INSERT INTO user_progress (user_id, manga_id, status, current_chapter, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, manga_id) DO UPDATE SET
			status=excluded.status,
			current_chapter=excluded.current_chapter,
			updated_at=excluded.updated_at
	`, userID, mangaID, status, chapter, time.Now())
	return err
}

func (r *Repository) UpdateProgress(userID, mangaID string, chapter int) error {
	res, err := r.db.Exec(`
		UPDATE user_progress SET current_chapter = ?, updated_at = ?
		WHERE user_id = ? AND manga_id = ?
	`, chapter, time.Now(), userID, mangaID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errors.New("manga not in user library")
	}
	return nil
}

func (r *Repository) UpdateRating(userID, mangaID string, rating int) error {
	_, err := r.db.Exec(`
		UPDATE user_progress SET rating = ?, updated_at = ?
		WHERE user_id = ? AND manga_id = ?
	`, rating, time.Now(), userID, mangaID)
	return err
}

func (r *Repository) RemoveFromLibrary(userID, mangaID string) error {
	_, err := r.db.Exec(`DELETE FROM user_progress WHERE user_id = ? AND manga_id = ?`, userID, mangaID)
	return err
}

func (r *Repository) GetLibrary(userID, statusFilter string) ([]*models.LibraryEntry, error) {
	q := `
		SELECT m.id, m.title, p.status, p.current_chapter, m.total_chapters, p.rating, p.updated_at
		FROM user_progress p
		JOIN manga m ON p.manga_id = m.id
		WHERE p.user_id = ?
	`
	args := []interface{}{userID}
	if statusFilter != "" {
		q += " AND p.status = ?"
		args = append(args, statusFilter)
	}
	q += " ORDER BY p.updated_at DESC"

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*models.LibraryEntry{}
	for rows.Next() {
		e := &models.LibraryEntry{}
		if err := rows.Scan(&e.MangaID, &e.Title, &e.Status, &e.CurrentChapter, &e.TotalChapters, &e.Rating, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) GetProgress(userID, mangaID string) (*models.UserProgress, error) {
	p := &models.UserProgress{}
	err := r.db.QueryRow(`
		SELECT user_id, manga_id, current_chapter, status, rating, updated_at
		FROM user_progress WHERE user_id = ? AND manga_id = ?
	`, userID, mangaID).Scan(&p.UserID, &p.MangaID, &p.CurrentChapter, &p.Status, &p.Rating, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, errors.New("progress not found")
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repository) Count() (int, error) {
	var c int
	err := r.db.QueryRow("SELECT COUNT(*) FROM manga").Scan(&c)
	return c, err
}
